// Package mcpserver exposes an investor.TradingAccount as a Model Context Protocol server.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "money-pies"
	serverVersion = "0.1.0"

	defaultRecentOrdersLimit = 10
	maxRecentOrdersLimit     = 100
)

// New creates an MCP server whose tools wrap the read-only part of the investor.TradingAccount interface.
func New(account investor.ReadOnlyTradingAccount) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)
	h := &handlers{account: account}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_account_summary",
		Description: "Get the trading account type and its balances: total cash, cash available for trading, cash available for withdrawal, long market value, short market value and pending deposits.",
		Annotations: &mcp.ToolAnnotations{Title: "Get account summary", ReadOnlyHint: true},
	}, h.accountSummary)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_positions",
		Description: "List the current positions held in the trading account.",
		Annotations: &mcp.ToolAnnotations{Title: "Get positions", ReadOnlyHint: true},
	}, h.positions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_latest_prices",
		Description: "Get the latest regular market price for each of the given ticker symbols.",
		Annotations: &mcp.ToolAnnotations{Title: "Get latest prices", ReadOnlyHint: true},
	}, h.latestPrices)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_order_status",
		Description: "Get the current status of an order by its ID.",
		Annotations: &mcp.ToolAnnotations{Title: "Get order status", ReadOnlyHint: true},
	}, h.orderStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recent_orders",
		Description: "List the most recent orders for the trading account.",
		Annotations: &mcp.ToolAnnotations{Title: "Get recent orders", ReadOnlyHint: true},
	}, h.recentOrders)

	return server
}

type handlers struct {
	account investor.ReadOnlyTradingAccount

	// summaryMu keeps concurrent get_account_summary calls from refreshing the
	// account while another call is reading its balances.
	summaryMu sync.Mutex
}

type emptyInput struct{}

type AccountSummary struct {
	Type                       string  `json:"type" jsonschema:"the type of the trading account"`
	TotalCash                  float64 `json:"total_cash" jsonschema:"total cash in the account"`
	CashAvailableForTrading    float64 `json:"cash_available_for_trading" jsonschema:"cash that can be used to place orders"`
	CashAvailableForWithdrawal float64 `json:"cash_available_for_withdrawal" jsonschema:"cash that can be withdrawn"`
	LongMarketValue            float64 `json:"long_market_value" jsonschema:"market value of long positions"`
	ShortMarketValue           float64 `json:"short_market_value" jsonschema:"market value of short positions"`
	PendingDeposits            float64 `json:"pending_deposits" jsonschema:"deposits that have not settled yet"`
}

type Position struct {
	Symbol          string  `json:"symbol"`
	Quantity        float64 `json:"quantity"`
	AveragePrice    float64 `json:"average_price"`
	CurrentPrice    float64 `json:"current_price"`
	MarketValue     float64 `json:"market_value"`
	UnrealizedPL    float64 `json:"unrealized_pl"`
	UnrealizedPLPct float64 `json:"unrealized_pl_pct"`
}

type PositionsOutput struct {
	Positions []Position `json:"positions"`
}

type LatestPricesInput struct {
	Symbols []string `json:"symbols" jsonschema:"ticker symbols to get prices for, e.g. AAPL"`
}

type LatestPricesOutput struct {
	Prices map[string]float64 `json:"prices" jsonschema:"latest regular market price keyed by symbol"`
}

type OrderIDInput struct {
	OrderID string `json:"order_id" jsonschema:"the brokerage's ID of the order"`
}

type RecentOrdersInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of orders to return, between 1 and 100 (default 10)"`
}

// Order is the tool representation of an investor.TradeOrder. The brokerage's raw response is left out.
type Order struct {
	ID          string   `json:"id"`
	Symbol      string   `json:"symbol"`
	Action      string   `json:"action"`
	Type        string   `json:"type"`
	Quantity    float64  `json:"quantity"`
	LimitPrice  *float64 `json:"limit_price,omitempty"`
	Status      string   `json:"status"`
	FilledQty   float64  `json:"filled_qty"`
	FilledPrice float64  `json:"filled_price"`
	SubmittedAt string   `json:"submitted_at,omitempty" jsonschema:"RFC 3339 timestamp"`
	FilledAt    string   `json:"filled_at,omitempty" jsonschema:"RFC 3339 timestamp"`
}

type RecentOrdersOutput struct {
	Orders []Order `json:"orders"`
}

func (h *handlers) accountSummary(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, AccountSummary, error) {
	h.summaryMu.Lock()
	defer h.summaryMu.Unlock()

	// The balances and type are only as fresh as the last refresh.
	var summary AccountSummary
	account := h.account
	err := account.RefreshAccount(ctx)
	if err != nil {
		return nil, summary, fmt.Errorf("failed to refresh account: %w", err)
	}

	if summary.Type, err = account.Type(ctx); err != nil {
		return nil, summary, fmt.Errorf("failed to get account type: %w", err)
	}

	balances := []struct {
		name string
		get  func(context.Context) (float64, error)
		dst  *float64
	}{
		{"total cash", account.TotalCash, &summary.TotalCash},
		{"cash available for trading", account.CashAvailableForTrading, &summary.CashAvailableForTrading},
		{"cash available for withdrawal", account.CashAvailableForWithdrawal, &summary.CashAvailableForWithdrawal},
		{"long market value", account.LongMarketValue, &summary.LongMarketValue},
		{"short market value", account.ShortMarketValue, &summary.ShortMarketValue},
		{"pending deposits", account.PendingDeposits, &summary.PendingDeposits},
	}
	for _, b := range balances {
		if *b.dst, err = b.get(ctx); err != nil {
			return nil, summary, fmt.Errorf("failed to get %s: %w", b.name, err)
		}
	}

	return nil, summary, nil
}

func (h *handlers) positions(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, PositionsOutput, error) {
	out := PositionsOutput{Positions: []Position{}}
	positions, err := h.account.Positions(ctx)
	if err != nil {
		return nil, out, err
	}

	for _, p := range positions {
		out.Positions = append(out.Positions, Position{
			Symbol:          p.Symbol,
			Quantity:        p.Quantity,
			AveragePrice:    p.AveragePrice,
			CurrentPrice:    p.CurrentPrice,
			MarketValue:     p.MarketValue,
			UnrealizedPL:    p.UnrealizedPL,
			UnrealizedPLPct: p.UnrealizedPLPct,
		})
	}
	return nil, out, nil
}

func (h *handlers) latestPrices(ctx context.Context, _ *mcp.CallToolRequest, in LatestPricesInput) (*mcp.CallToolResult, LatestPricesOutput, error) {
	out := LatestPricesOutput{Prices: map[string]float64{}}

	symbols := make([]string, 0, len(in.Symbols))
	for _, s := range in.Symbols {
		if s = normalizeSymbol(s); s != "" {
			symbols = append(symbols, s)
		}
	}
	if len(symbols) == 0 {
		return nil, out, errors.New("at least one symbol is required")
	}

	prices, err := h.account.LatestRegularMarketPrices(ctx, symbols)
	if err != nil {
		return nil, out, err
	}
	if prices != nil {
		out.Prices = prices
	}
	return nil, out, nil
}

func (h *handlers) orderStatus(ctx context.Context, _ *mcp.CallToolRequest, in OrderIDInput) (*mcp.CallToolResult, Order, error) {
	orderID := strings.TrimSpace(in.OrderID)
	if orderID == "" {
		return nil, Order{}, errors.New("order_id is required")
	}

	order, err := h.account.GetOrderStatus(ctx, orderID)
	if err != nil {
		return nil, Order{}, err
	}
	if order == nil {
		return nil, Order{}, errors.New("brokerage returned no order")
	}
	return nil, toOrder(*order), nil
}

func (h *handlers) recentOrders(ctx context.Context, _ *mcp.CallToolRequest, in RecentOrdersInput) (*mcp.CallToolResult, RecentOrdersOutput, error) {
	out := RecentOrdersOutput{Orders: []Order{}}

	limit := in.Limit
	if limit == 0 {
		limit = defaultRecentOrdersLimit
	}
	if limit < 1 || limit > maxRecentOrdersLimit {
		return nil, out, fmt.Errorf("limit must be between 1 and %d", maxRecentOrdersLimit)
	}

	orders, err := h.account.GetRecentOrders(ctx, limit)
	if err != nil {
		return nil, out, err
	}
	for _, o := range orders {
		out.Orders = append(out.Orders, toOrder(o))
	}
	return nil, out, nil
}

func toOrder(o investor.TradeOrder) Order {
	order := Order{
		ID:          o.ID,
		Symbol:      o.Symbol,
		Action:      string(o.Action),
		Type:        string(o.Type),
		Quantity:    o.Quantity,
		LimitPrice:  o.LimitPrice,
		Status:      string(o.Status),
		FilledQty:   o.FilledQty,
		FilledPrice: o.FilledPrice,
	}
	if !o.SubmittedAt.IsZero() {
		order.SubmittedAt = o.SubmittedAt.Format(time.RFC3339)
	}
	if o.FilledAt != nil && !o.FilledAt.IsZero() {
		order.FilledAt = o.FilledAt.Format(time.RFC3339)
	}
	return order
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
