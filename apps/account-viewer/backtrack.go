package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/backtracker"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

type backtrackPointJSON struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type backtrackSliceJSON struct {
	Symbol         string  `json:"symbol"`
	Weight         float64 `json:"weight"`
	StartPrice     float64 `json:"start_price"`
	EndPrice       float64 `json:"end_price"`
	PriceReturnPct float64 `json:"price_return_pct"`
	Contribution   float64 `json:"contribution"`
}

type backtrackExcludedJSON struct {
	Symbol string  `json:"symbol"`
	Weight float64 `json:"weight"`
	Reason string  `json:"reason"`
}

type backtrackJSON struct {
	Years               int                     `json:"years"`
	Rebalance           string                  `json:"rebalance"`
	From                string                  `json:"from"`
	To                  string                  `json:"to"`
	InitialAmount       float64                 `json:"initial_amount"`
	FinalAmount         float64                 `json:"final_amount"`
	TotalReturnPct      float64                 `json:"total_return_pct"`
	AnnualizedReturnPct float64                 `json:"annualized_return_pct"`
	MaxDrawdownPct      float64                 `json:"max_drawdown_pct"`
	Points              []backtrackPointJSON    `json:"points"`
	Slices              []backtrackSliceJSON    `json:"slices"`
	Excluded            []backtrackExcludedJSON `json:"excluded"`
}

// backtrack simulates holding the account's current positions, at their current weights,
// from the start of the requested period.
func (h *handler) backtrack(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	yearsParam := query.Get("years")
	if yearsParam == "" {
		yearsParam = "1"
	}
	years, ok := backtrackYears[yearsParam]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "years must be 1, 3 or 5"})
		return
	}

	rebalance := backtracker.Rebalance(query.Get("rebalance"))
	switch rebalance {
	case "":
		rebalance = backtracker.RebalanceNever
	case backtracker.RebalanceNever, backtracker.RebalanceMonthly, backtracker.RebalanceQuarterly, backtracker.RebalanceYearly:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rebalance must be never, monthly, quarterly or yearly"})
		return
	}

	h.backtrackMu.Lock()
	defer h.backtrackMu.Unlock()

	ctx := r.Context()
	positions, err := h.account.Positions(ctx)
	if err != nil {
		writeError(w, fmt.Errorf("failed to get positions: %w", err))
		return
	}

	pie, err := investor.PieFromPositions(positions)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	to := time.Now().UTC()
	result, err := backtracker.Backtrack(ctx, h.prices, pie, backtracker.Options{
		From:               to.AddDate(-years, 0, 0),
		To:                 to,
		InitialAmount:      backtrackAmount,
		Rebalance:          rebalance,
		SkipMissingHistory: true,
	})
	if err != nil {
		writeError(w, fmt.Errorf("failed to backtrack: %w", err))
		return
	}

	response := backtrackJSON{
		Years:               years,
		Rebalance:           string(result.Rebalance),
		From:                result.From.Format(time.DateOnly),
		To:                  result.To.Format(time.DateOnly),
		InitialAmount:       result.InitialAmount,
		FinalAmount:         result.FinalAmount,
		TotalReturnPct:      result.TotalReturnPct,
		AnnualizedReturnPct: result.AnnualizedReturnPct,
		MaxDrawdownPct:      result.MaxDrawdownPct,
		Points:              make([]backtrackPointJSON, 0, len(result.Points)),
		Slices:              make([]backtrackSliceJSON, 0, len(result.Slices)),
		Excluded:            make([]backtrackExcludedJSON, 0, len(result.Excluded)),
	}
	for _, point := range result.Points {
		response.Points = append(response.Points, backtrackPointJSON{Date: point.Date.Format(time.DateOnly), Value: point.Value})
	}
	for _, slice := range result.Slices {
		response.Slices = append(response.Slices, backtrackSliceJSON(slice))
	}
	for _, excluded := range result.Excluded {
		response.Excluded = append(response.Excluded, backtrackExcludedJSON(excluded))
	}

	writeJSON(w, http.StatusOK, response)
}

// cachedPrices is a backtracker.PriceSource that asks the brokerage for a symbol's
// history once per day and period.
type cachedPrices struct {
	source backtracker.PriceSource

	mu        sync.Mutex
	histories map[string][]investor.DailyPrice
}

func (c *cachedPrices) DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error) {
	key := fmt.Sprintf("%s|%s|%s", symbol, from.Format(time.DateOnly), to.Format(time.DateOnly))

	c.mu.Lock()
	history, ok := c.histories[key]
	c.mu.Unlock()
	if ok {
		return history, nil
	}

	history, err := c.source.DailyClosingPrices(ctx, symbol, from, to)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.histories[key] = history
	c.mu.Unlock()
	return history, nil
}
