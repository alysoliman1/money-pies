// Package backtracker simulates how a pie would have performed over a past period.
package backtracker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

const (
	// startTolerance is how long after the requested start a symbol's first price may be
	// before the symbol counts as having no history for the period. It covers weekends and holidays.
	startTolerance = 10 * 24 * time.Hour

	// maxConcurrentRequests bounds how many price histories are requested at once.
	maxConcurrentRequests = 5

	daysPerYear = 365.25
)

// PriceSource provides the historical prices the simulation runs on.
// Every investor.ReadOnlyTradingAccount is a PriceSource.
type PriceSource interface {
	// DailyClosingPrices retrieves the split-adjusted closing price of the symbol for every
	// trading day between from and to. A symbol without prices in the range yields no prices and no error.
	DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error)
}

// Rebalance is how often the holdings are brought back to the pie's weights.
type Rebalance string

const (
	// RebalanceNever buys the pie once at the start and holds it.
	RebalanceNever     Rebalance = "never"
	RebalanceMonthly   Rebalance = "monthly"
	RebalanceQuarterly Rebalance = "quarterly"
	RebalanceYearly    Rebalance = "yearly"
)

// Options configures a simulation.
type Options struct {
	// From and To bound the simulated period.
	From time.Time
	To   time.Time

	// InitialAmount is invested according to the pie's weights on the first trading day.
	InitialAmount float64

	// Rebalance defaults to RebalanceNever.
	Rebalance Rebalance

	// SkipMissingHistory leaves out slices that have no prices at the start of the period
	// and spreads their weight over the remaining slices in proportion. The left out slices
	// are reported in Result.Excluded. When false, such a slice fails the simulation.
	SkipMissingHistory bool
}

// Point is the value of the simulated holdings at the close of a trading day.
type Point struct {
	Date  time.Time
	Value float64
}

// SliceResult is how one slice of the pie did over the period.
type SliceResult struct {
	Symbol string
	// Weight is the fraction of the pie the slice was simulated with. It differs from
	// the pie's weight when other slices were excluded.
	Weight     float64
	StartPrice float64
	EndPrice   float64
	// PriceReturnPct is the change in the slice's price over the period.
	PriceReturnPct float64
	// Contribution is the amount the slice added to (or took from) the final amount.
	// The contributions of all slices add up to FinalAmount - InitialAmount.
	Contribution float64
}

// ExcludedSlice is a slice that was left out of the simulation.
type ExcludedSlice struct {
	Symbol string
	// Weight is the fraction of the pie the slice has.
	Weight float64
	Reason string
}

// Result is the outcome of a simulation.
type Result struct {
	// From and To are the first and last trading day that were simulated.
	From          time.Time
	To            time.Time
	InitialAmount float64
	FinalAmount   float64
	Rebalance     Rebalance

	TotalReturnPct float64
	// AnnualizedReturnPct is the yearly growth rate that compounds to TotalReturnPct.
	// For periods much shorter than a year it extrapolates and is not meaningful.
	AnnualizedReturnPct float64
	// MaxDrawdownPct is the largest drop from a peak to a later low, as a positive percentage.
	MaxDrawdownPct float64

	Points   []Point
	Slices   []SliceResult
	Excluded []ExcludedSlice
}

// Backtrack simulates investing options.InitialAmount in the pie at options.From and holding
// it until options.To, using closing prices from the price source.
//
// Prices are adjusted for splits but not for dividends, so the result is price return only:
// it leaves out dividends, as well as fees and taxes. Spin-offs are not adjusted for either,
// so a slice that spun off part of itself shows a drop it did not really have.
// Shares are fractional.
func Backtrack(ctx context.Context, prices PriceSource, pie investor.Pie, options Options) (*Result, error) {
	if prices == nil {
		return nil, errors.New("price source is nil")
	}

	if options.Rebalance == "" {
		options.Rebalance = RebalanceNever
	}
	if err := validateOptions(options); err != nil {
		return nil, err
	}

	weights, symbols, err := pie.ValidatePie()
	if err != nil {
		return nil, fmt.Errorf("failed to validate pie: %w", err)
	}

	histories, err := fetchHistories(ctx, prices, symbols, options.From, options.To)
	if err != nil {
		return nil, err
	}

	symbols, excluded := splitByHistory(symbols, weights, histories, options.From)
	if len(excluded) > 0 && !options.SkipMissingHistory {
		names := make([]string, 0, len(excluded))
		for _, e := range excluded {
			names = append(names, e.Symbol)
		}
		return nil, fmt.Errorf("no price history at the start of the period for: %s", strings.Join(names, ", "))
	}
	if len(symbols) == 0 {
		return nil, errors.New("no slice of the pie has price history at the start of the period")
	}

	// Spread the weight of the excluded slices over the ones that remain.
	includedWeight := 0.0
	for _, symbol := range symbols {
		includedWeight += weights[symbol]
	}
	simulatedWeights := make([]float64, len(symbols))
	for i, symbol := range symbols {
		simulatedWeights[i] = weights[symbol] / includedWeight
	}

	dates, closes := alignPrices(symbols, histories)
	if len(dates) < 2 {
		return nil, errors.New("fewer than two trading days have prices for every slice in the period")
	}

	result := simulate(symbols, simulatedWeights, dates, closes, options)
	result.Excluded = excluded
	return result, nil
}

func validateOptions(options Options) error {
	if options.From.IsZero() || options.To.IsZero() {
		return errors.New("the period to simulate is not specified")
	}
	if !options.From.Before(options.To) {
		return errors.New("the start of the period must be before its end")
	}
	if options.InitialAmount <= 0 || math.IsNaN(options.InitialAmount) || math.IsInf(options.InitialAmount, 0) {
		return fmt.Errorf("initial amount ($%f) is not valid", options.InitialAmount)
	}
	switch options.Rebalance {
	case RebalanceNever, RebalanceMonthly, RebalanceQuarterly, RebalanceYearly:
		return nil
	default:
		return fmt.Errorf("rebalance %q is not valid", options.Rebalance)
	}
}

// fetchHistories retrieves the price history of every symbol, keeping only usable prices.
func fetchHistories(ctx context.Context, prices PriceSource, symbols []string, from, to time.Time) (map[string][]investor.DailyPrice, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		firstErr  error
		histories = make(map[string][]investor.DailyPrice, len(symbols))
		slots     = make(chan struct{}, maxConcurrentRequests)
	)

	for _, symbol := range symbols {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			if ctx.Err() != nil {
				return
			}
			history, err := prices.DailyClosingPrices(ctx, symbol, from, to)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to get prices for %s: %w", symbol, err)
					cancel()
				}
				return
			}
			histories[symbol] = cleanHistory(history, from, to)
		})
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return histories, nil
}

// cleanHistory keeps the valid prices inside the period, one per day, oldest first.
func cleanHistory(history []investor.DailyPrice, from, to time.Time) []investor.DailyPrice {
	firstDay, lastDay := day(from), day(to)

	byDay := make(map[time.Time]float64, len(history))
	for _, price := range history {
		d := day(price.Date)
		if d.Before(firstDay) || d.After(lastDay) {
			continue
		}
		if price.Close <= 0 || math.IsNaN(price.Close) || math.IsInf(price.Close, 0) {
			continue
		}
		byDay[d] = price.Close
	}

	cleaned := make([]investor.DailyPrice, 0, len(byDay))
	for d, closePrice := range byDay {
		cleaned = append(cleaned, investor.DailyPrice{Date: d, Close: closePrice})
	}
	sort.Slice(cleaned, func(i, j int) bool { return cleaned[i].Date.Before(cleaned[j].Date) })
	return cleaned
}

// splitByHistory separates the symbols that have prices from the start of the period from those that do not.
func splitByHistory(
	symbols []string,
	weights map[string]float64,
	histories map[string][]investor.DailyPrice,
	from time.Time,
) ([]string, []ExcludedSlice) {
	included := make([]string, 0, len(symbols))
	var excluded []ExcludedSlice

	latestStart := day(from).Add(startTolerance)
	for _, symbol := range symbols {
		history := histories[symbol]
		switch {
		case len(history) == 0:
			excluded = append(excluded, ExcludedSlice{
				Symbol: symbol,
				Weight: weights[symbol],
				Reason: "no prices in the period",
			})
		case history[0].Date.After(latestStart):
			excluded = append(excluded, ExcludedSlice{
				Symbol: symbol,
				Weight: weights[symbol],
				Reason: fmt.Sprintf("prices only start on %s", history[0].Date.Format(time.DateOnly)),
			})
		default:
			included = append(included, symbol)
		}
	}
	return included, excluded
}

// alignPrices returns the trading days on which every symbol has a price, oldest first,
// and for each symbol its closing prices on those days.
func alignPrices(symbols []string, histories map[string][]investor.DailyPrice) ([]time.Time, [][]float64) {
	daysWithPrice := map[time.Time]int{}
	for _, symbol := range symbols {
		for _, price := range histories[symbol] {
			daysWithPrice[price.Date]++
		}
	}

	dates := make([]time.Time, 0, len(daysWithPrice))
	for d, count := range daysWithPrice {
		if count == len(symbols) {
			dates = append(dates, d)
		}
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	index := make(map[time.Time]int, len(dates))
	for i, d := range dates {
		index[d] = i
	}

	closes := make([][]float64, len(symbols))
	for s, symbol := range symbols {
		closes[s] = make([]float64, len(dates))
		for _, price := range histories[symbol] {
			if i, ok := index[price.Date]; ok {
				closes[s][i] = price.Close
			}
		}
	}
	return dates, closes
}

// simulate buys the weights on the first day and values the holdings on every later day.
// closes[s][d] is the closing price of symbols[s] on dates[d], and weights add up to 1.
func simulate(symbols []string, weights []float64, dates []time.Time, closes [][]float64, options Options) *Result {
	shares := make([]float64, len(symbols))
	contributions := make([]float64, len(symbols))
	for s := range symbols {
		shares[s] = options.InitialAmount * weights[s] / closes[s][0]
	}

	points := make([]Point, 0, len(dates))
	points = append(points, Point{Date: dates[0], Value: options.InitialAmount})

	peak, maxDrawdown := options.InitialAmount, 0.0
	for d := 1; d < len(dates); d++ {
		value := 0.0
		for s := range symbols {
			contributions[s] += shares[s] * (closes[s][d] - closes[s][d-1])
			value += shares[s] * closes[s][d]
		}
		points = append(points, Point{Date: dates[d], Value: value})

		peak = math.Max(peak, value)
		maxDrawdown = math.Max(maxDrawdown, (peak-value)/peak)

		// Rebalance at the close of the first trading day of a new period.
		if rebalanceDue(options.Rebalance, dates[d-1], dates[d]) {
			for s := range symbols {
				shares[s] = value * weights[s] / closes[s][d]
			}
		}
	}

	last := len(dates) - 1
	final := points[last].Value
	growth := final / options.InitialAmount
	years := dates[last].Sub(dates[0]).Hours() / 24 / daysPerYear

	slices := make([]SliceResult, len(symbols))
	for s, symbol := range symbols {
		slices[s] = SliceResult{
			Symbol:         symbol,
			Weight:         weights[s],
			StartPrice:     closes[s][0],
			EndPrice:       closes[s][last],
			PriceReturnPct: (closes[s][last]/closes[s][0] - 1) * 100,
			Contribution:   contributions[s],
		}
	}

	return &Result{
		From:                dates[0],
		To:                  dates[last],
		InitialAmount:       options.InitialAmount,
		FinalAmount:         final,
		Rebalance:           options.Rebalance,
		TotalReturnPct:      (growth - 1) * 100,
		AnnualizedReturnPct: (math.Pow(growth, 1/years) - 1) * 100,
		MaxDrawdownPct:      maxDrawdown * 100,
		Points:              points,
		Slices:              slices,
	}
}

// rebalanceDue reports whether current is the first trading day of a new rebalancing period.
func rebalanceDue(rebalance Rebalance, previous, current time.Time) bool {
	switch rebalance {
	case RebalanceMonthly:
		return current.Year() != previous.Year() || current.Month() != previous.Month()
	case RebalanceQuarterly:
		return current.Year() != previous.Year() || quarter(current) != quarter(previous)
	case RebalanceYearly:
		return current.Year() != previous.Year()
	default:
		return false
	}
}

func quarter(t time.Time) int {
	return (int(t.Month()) - 1) / 3
}

// day truncates a time to its calendar day in UTC.
func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
