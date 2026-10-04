package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

//go:embed web
var webFiles embed.FS

const (
	recentOrdersLimit = 25

	// backtrackAmount is the amount every backtrack starts with, so that runs are comparable.
	backtrackAmount = 10000
)

// backtrackYears are the periods the page offers.
var backtrackYears = map[string]int{"1": 1, "3": 3, "5": 5}

type handler struct {
	account   investor.ReadOnlyTradingAccount
	brokerage string
	static    http.Handler

	// mu keeps concurrent snapshots from refreshing the account while another
	// request is reading its balances.
	mu sync.Mutex

	// prices remembers price histories, so that running the backtrack again with
	// another setting does not request them all from the brokerage again.
	prices *cachedPrices
	// backtrackMu runs one backtrack at a time; each makes a request per position.
	backtrackMu sync.Mutex
}

// newHandler serves the viewer page and its JSON API for the given account.
func newHandler(account investor.ReadOnlyTradingAccount, brokerage string) http.Handler {
	web, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}

	h := &handler{
		account:   account,
		brokerage: brokerage,
		static:    http.FileServer(http.FS(web)),
		prices:    &cachedPrices{source: account, histories: map[string][]investor.DailyPrice{}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", h.snapshot)
	mux.HandleFunc("/api/backtrack", h.backtrack)
	mux.Handle("/", h.static)
	return h.guard(mux)
}

// guard only lets read requests addressed to this machine through.
func (h *handler) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A page on another site could otherwise reach this server through a DNS name
		// that resolves to 127.0.0.1 and read the account data.
		host := r.Host
		if hostOnly, _, err := net.SplitHostPort(r.Host); err == nil {
			host = hostOnly
		}
		if !isLoopbackHost(host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type summaryJSON struct {
	Type                       string  `json:"type"`
	TotalCash                  float64 `json:"total_cash"`
	CashAvailableForTrading    float64 `json:"cash_available_for_trading"`
	CashAvailableForWithdrawal float64 `json:"cash_available_for_withdrawal"`
	LongMarketValue            float64 `json:"long_market_value"`
	ShortMarketValue           float64 `json:"short_market_value"`
	PendingDeposits            float64 `json:"pending_deposits"`
}

type positionJSON struct {
	Symbol          string  `json:"symbol"`
	Quantity        float64 `json:"quantity"`
	AveragePrice    float64 `json:"average_price"`
	CurrentPrice    float64 `json:"current_price"`
	MarketValue     float64 `json:"market_value"`
	UnrealizedPL    float64 `json:"unrealized_pl"`
	UnrealizedPLPct float64 `json:"unrealized_pl_pct"`
}

type orderJSON struct {
	ID          string     `json:"id"`
	Symbol      string     `json:"symbol"`
	Action      string     `json:"action"`
	Type        string     `json:"type"`
	Quantity    float64    `json:"quantity"`
	LimitPrice  *float64   `json:"limit_price,omitempty"`
	Status      string     `json:"status"`
	FilledQty   float64    `json:"filled_qty"`
	FilledPrice float64    `json:"filled_price"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	FilledAt    *time.Time `json:"filled_at,omitempty"`
}

type snapshotJSON struct {
	Brokerage string         `json:"brokerage"`
	FetchedAt time.Time      `json:"fetched_at"`
	Summary   summaryJSON    `json:"summary"`
	Positions []positionJSON `json:"positions"`
	Orders    []orderJSON    `json:"orders"`
	// OrdersError is set when the balances and positions loaded but the orders did not.
	OrdersError string `json:"orders_error,omitempty"`
}

func (h *handler) snapshot(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ctx := r.Context()
	snapshot := snapshotJSON{
		Brokerage: h.brokerage,
		FetchedAt: time.Now(),
		Positions: []positionJSON{},
		Orders:    []orderJSON{},
	}

	summary, err := h.summary(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	snapshot.Summary = summary

	positions, err := h.account.Positions(ctx)
	if err != nil {
		writeError(w, fmt.Errorf("failed to get positions: %w", err))
		return
	}
	for _, p := range positions {
		snapshot.Positions = append(snapshot.Positions, positionJSON{
			Symbol:          p.Symbol,
			Quantity:        p.Quantity,
			AveragePrice:    p.AveragePrice,
			CurrentPrice:    p.CurrentPrice,
			MarketValue:     p.MarketValue,
			UnrealizedPL:    p.UnrealizedPL,
			UnrealizedPLPct: p.UnrealizedPLPct,
		})
	}

	// The orders are secondary: the rest of the page is still useful without them.
	orders, err := h.account.GetRecentOrders(ctx, recentOrdersLimit)
	if err != nil {
		snapshot.OrdersError = err.Error()
	}
	for _, o := range orders {
		order := orderJSON{
			ID:          o.ID,
			Symbol:      o.Symbol,
			Action:      string(o.Action),
			Type:        string(o.Type),
			Quantity:    o.Quantity,
			LimitPrice:  o.LimitPrice,
			Status:      string(o.Status),
			FilledQty:   o.FilledQty,
			FilledPrice: o.FilledPrice,
			FilledAt:    o.FilledAt,
		}
		if !o.SubmittedAt.IsZero() {
			submittedAt := o.SubmittedAt
			order.SubmittedAt = &submittedAt
		}
		snapshot.Orders = append(snapshot.Orders, order)
	}

	writeJSON(w, http.StatusOK, snapshot)
}

func (h *handler) summary(ctx context.Context) (summaryJSON, error) {
	var summary summaryJSON

	// The balances and type are only as fresh as the last refresh.
	err := h.account.RefreshAccount(ctx)
	if err != nil {
		return summary, fmt.Errorf("failed to refresh account: %w", err)
	}

	if summary.Type, err = h.account.Type(ctx); err != nil {
		return summary, fmt.Errorf("failed to get account type: %w", err)
	}

	balances := []struct {
		name string
		get  func(context.Context) (float64, error)
		dst  *float64
	}{
		{"total cash", h.account.TotalCash, &summary.TotalCash},
		{"cash available for trading", h.account.CashAvailableForTrading, &summary.CashAvailableForTrading},
		{"cash available for withdrawal", h.account.CashAvailableForWithdrawal, &summary.CashAvailableForWithdrawal},
		{"long market value", h.account.LongMarketValue, &summary.LongMarketValue},
		{"short market value", h.account.ShortMarketValue, &summary.ShortMarketValue},
		{"pending deposits", h.account.PendingDeposits, &summary.PendingDeposits},
	}
	for _, b := range balances {
		if *b.dst, err = b.get(ctx); err != nil {
			return summary, fmt.Errorf("failed to get %s: %w", b.name, err)
		}
	}

	return summary, nil
}

func writeError(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
