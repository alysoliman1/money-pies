package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/backtracker"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultBacktrackYears  = 1
	maxBacktrackYears      = 20
	defaultBacktrackAmount = 10000
)

type BacktrackSliceInput struct {
	Symbol string  `json:"symbol" jsonschema:"ticker symbol, e.g. AAPL"`
	Weight float64 `json:"weight" jsonschema:"relative weight, must be greater than zero. Weights are scaled to add up to 100%, so percentages, dollar amounts or ratios all work"`
}

type BacktrackInput struct {
	Slices        []BacktrackSliceInput `json:"slices,omitempty" jsonschema:"the holdings to simulate, at least two. Leave out to simulate the account's current positions at their current weights"`
	Years         int                   `json:"years,omitempty" jsonschema:"how many years back the simulation starts, between 1 and 20 (default 1)"`
	Rebalance     string                `json:"rebalance,omitempty" jsonschema:"how often the holdings are brought back to the weights: never, monthly, quarterly or yearly (default never, which buys once and holds)"`
	InitialAmount float64               `json:"initial_amount,omitempty" jsonschema:"amount invested at the start (default 10000)"`
}

type BacktrackPoint struct {
	Date  string  `json:"date" jsonschema:"trading day, YYYY-MM-DD"`
	Value float64 `json:"value"`
}

type BacktrackSlice struct {
	Symbol         string  `json:"symbol"`
	WeightPct      float64 `json:"weight_pct" jsonschema:"weight the slice was simulated with, after scaling and after spreading the weight of excluded slices"`
	StartPrice     float64 `json:"start_price"`
	EndPrice       float64 `json:"end_price"`
	PriceReturnPct float64 `json:"price_return_pct"`
	Contribution   float64 `json:"contribution" jsonschema:"amount the slice added to or took from the final amount; all contributions add up to final_amount minus initial_amount"`
}

type BacktrackExcluded struct {
	Symbol    string  `json:"symbol"`
	WeightPct float64 `json:"weight_pct" jsonschema:"weight the slice had before it was left out"`
	Reason    string  `json:"reason"`
}

type BacktrackOutput struct {
	Source              string              `json:"source" jsonschema:"current_positions or given_slices"`
	From                string              `json:"from" jsonschema:"first trading day simulated, YYYY-MM-DD"`
	To                  string              `json:"to" jsonschema:"last trading day simulated, YYYY-MM-DD"`
	Rebalance           string              `json:"rebalance"`
	InitialAmount       float64             `json:"initial_amount"`
	FinalAmount         float64             `json:"final_amount"`
	TotalReturnPct      float64             `json:"total_return_pct"`
	AnnualizedReturnPct float64             `json:"annualized_return_pct" jsonschema:"yearly growth rate that compounds to the total return"`
	MaxDrawdownPct      float64             `json:"max_drawdown_pct" jsonschema:"largest drop from a peak to a later low, as a positive percentage"`
	MonthEndValues      []BacktrackPoint    `json:"month_end_values" jsonschema:"value on the first simulated day and on the last trading day of every month"`
	Slices              []BacktrackSlice    `json:"slices"`
	Excluded            []BacktrackExcluded `json:"excluded" jsonschema:"slices left out because they had no price history at the start of the period"`
}

func (h *handlers) backtrack(ctx context.Context, _ *mcp.CallToolRequest, in BacktrackInput) (*mcp.CallToolResult, BacktrackOutput, error) {
	out := BacktrackOutput{
		MonthEndValues: []BacktrackPoint{},
		Slices:         []BacktrackSlice{},
		Excluded:       []BacktrackExcluded{},
	}

	years := in.Years
	if years == 0 {
		years = defaultBacktrackYears
	}
	if years < 1 || years > maxBacktrackYears {
		return nil, out, fmt.Errorf("years must be between 1 and %d", maxBacktrackYears)
	}

	rebalance := backtracker.Rebalance(strings.ToLower(strings.TrimSpace(in.Rebalance)))
	switch rebalance {
	case "":
		rebalance = backtracker.RebalanceNever
	case backtracker.RebalanceNever, backtracker.RebalanceMonthly, backtracker.RebalanceQuarterly, backtracker.RebalanceYearly:
	default:
		return nil, out, errors.New("rebalance must be never, monthly, quarterly or yearly")
	}

	amount := in.InitialAmount
	if amount == 0 {
		amount = defaultBacktrackAmount
	}
	if amount < 0 {
		return nil, out, errors.New("initial_amount must be greater than zero")
	}

	var pie investor.Pie
	if len(in.Slices) > 0 {
		out.Source = "given_slices"
		var err error
		if pie, err = pieFromSlices(in.Slices); err != nil {
			return nil, out, err
		}
	} else {
		out.Source = "current_positions"
		positions, err := h.account.Positions(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("failed to get positions: %w", err)
		}
		if pie, err = investor.PieFromPositions(positions); err != nil {
			return nil, out, err
		}
	}

	to := time.Now().UTC()
	result, err := backtracker.Backtrack(ctx, h.account, pie, backtracker.Options{
		From:               to.AddDate(-years, 0, 0),
		To:                 to,
		InitialAmount:      amount,
		Rebalance:          rebalance,
		SkipMissingHistory: true,
	})
	if err != nil {
		return nil, out, err
	}

	out.From = result.From.Format(time.DateOnly)
	out.To = result.To.Format(time.DateOnly)
	out.Rebalance = string(result.Rebalance)
	out.InitialAmount = result.InitialAmount
	out.FinalAmount = result.FinalAmount
	out.TotalReturnPct = result.TotalReturnPct
	out.AnnualizedReturnPct = result.AnnualizedReturnPct
	out.MaxDrawdownPct = result.MaxDrawdownPct

	// Daily values would flood the caller, so only the month ends are returned.
	for i, point := range result.Points {
		last := i == len(result.Points)-1
		if i == 0 || last || result.Points[i+1].Date.Month() != point.Date.Month() {
			out.MonthEndValues = append(out.MonthEndValues, BacktrackPoint{Date: point.Date.Format(time.DateOnly), Value: point.Value})
		}
	}
	for _, slice := range result.Slices {
		out.Slices = append(out.Slices, BacktrackSlice{
			Symbol:         slice.Symbol,
			WeightPct:      slice.Weight * 100,
			StartPrice:     slice.StartPrice,
			EndPrice:       slice.EndPrice,
			PriceReturnPct: slice.PriceReturnPct,
			Contribution:   slice.Contribution,
		})
	}
	for _, excluded := range result.Excluded {
		out.Excluded = append(out.Excluded, BacktrackExcluded{
			Symbol:    excluded.Symbol,
			WeightPct: excluded.Weight * 100,
			Reason:    excluded.Reason,
		})
	}

	return nil, out, nil
}

// pieFromSlices builds a pie from relative weights, scaling them to add up to 100.
func pieFromSlices(slices []BacktrackSliceInput) (investor.Pie, error) {
	if len(slices) < 2 {
		return investor.Pie{}, errors.New("slices must hold at least two symbols")
	}

	total := 0.0
	for _, slice := range slices {
		if normalizeSymbol(slice.Symbol) == "" {
			return investor.Pie{}, errors.New("every slice needs a symbol")
		}
		if slice.Weight <= 0 {
			return investor.Pie{}, fmt.Errorf("weight for %s must be greater than zero", normalizeSymbol(slice.Symbol))
		}
		total += slice.Weight
	}

	pie := investor.Pie{Name: "Given slices"}
	for _, slice := range slices {
		pie.Slices = append(pie.Slices, investor.Slice{
			Symbol: normalizeSymbol(slice.Symbol),
			Weight: slice.Weight / total * 100,
		})
	}
	return pie, nil
}
