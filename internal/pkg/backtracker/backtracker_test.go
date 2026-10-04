package backtracker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func date(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

// fakePrices serves fixed price histories.
type fakePrices struct {
	mu        sync.Mutex
	histories map[string][]investor.DailyPrice
	errs      map[string]error
	requested []string
}

func (f *fakePrices) DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requested = append(f.requested, symbol)
	return f.histories[symbol], f.errs[symbol]
}

// history builds a price history from closes on the given days.
func history(days []time.Time, closes ...float64) []investor.DailyPrice {
	prices := make([]investor.DailyPrice, len(closes))
	for i, c := range closes {
		prices[i] = investor.DailyPrice{Date: days[i], Close: c}
	}
	return prices
}

func pieOf(slices ...investor.Slice) investor.Pie {
	return investor.Pie{Slices: slices}
}

var (
	threeDays = []time.Time{date(2025, 1, 2), date(2025, 1, 3), date(2025, 1, 6)}
	evenPie   = pieOf(investor.Slice{Symbol: "A", Weight: 50}, investor.Slice{Symbol: "B", Weight: 50})
	january   = Options{From: date(2025, 1, 1), To: date(2025, 1, 31), InitialAmount: 1000}
)

func TestBacktrackBuyAndHold(t *testing.T) {
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(threeDays, 100, 110, 120),
		"B": history(threeDays, 50, 40, 45),
	}}

	result, err := Backtrack(context.Background(), prices, evenPie, january)

	require.NoError(t, err)
	// $500 buys 5 shares of A and 10 shares of B.
	assert.Equal(t, []Point{
		{Date: date(2025, 1, 2), Value: 1000},
		{Date: date(2025, 1, 3), Value: 950},
		{Date: date(2025, 1, 6), Value: 1050},
	}, result.Points)

	assert.Equal(t, date(2025, 1, 2), result.From)
	assert.Equal(t, date(2025, 1, 6), result.To)
	assert.Equal(t, 1000.0, result.InitialAmount)
	assert.Equal(t, 1050.0, result.FinalAmount)
	assert.Equal(t, RebalanceNever, result.Rebalance)
	assert.InDelta(t, 5, result.TotalReturnPct, 1e-9)
	assert.InDelta(t, 5, result.MaxDrawdownPct, 1e-9, "the drop from 1000 to 950")
	assert.Empty(t, result.Excluded)

	require.Len(t, result.Slices, 2)
	a, b := result.Slices[0], result.Slices[1]
	assert.Equal(t, "A", a.Symbol)
	assert.Equal(t, 0.5, a.Weight)
	assert.Equal(t, 100.0, a.StartPrice)
	assert.Equal(t, 120.0, a.EndPrice)
	assert.InDelta(t, 20, a.PriceReturnPct, 1e-9)
	assert.InDelta(t, 100, a.Contribution, 1e-9)
	assert.InDelta(t, -10, b.PriceReturnPct, 1e-9)
	assert.InDelta(t, -50, b.Contribution, 1e-9)
	assert.InDelta(t, result.FinalAmount-result.InitialAmount, a.Contribution+b.Contribution, 1e-9)
}

func TestBacktrackUsesThePieWeights(t *testing.T) {
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(threeDays, 100, 100, 200),
		"B": history(threeDays, 50, 50, 50),
	}}
	pie := pieOf(investor.Slice{Symbol: "A", Weight: 20}, investor.Slice{Symbol: "B", Weight: 80})

	result, err := Backtrack(context.Background(), prices, pie, january)

	require.NoError(t, err)
	// Only the 20% in A doubles.
	assert.InDelta(t, 1200, result.FinalAmount, 1e-9)
}

func TestBacktrackRebalance(t *testing.T) {
	days := []time.Time{date(2025, 1, 30), date(2025, 1, 31), date(2025, 2, 3), date(2025, 2, 4)}
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(days, 100, 200, 200, 100),
		"B": history(days, 100, 100, 100, 100),
	}}
	options := Options{From: date(2025, 1, 30), To: date(2025, 2, 28), InitialAmount: 1000}

	held, err := Backtrack(context.Background(), prices, evenPie, options)
	require.NoError(t, err)
	// 5 shares of each are held throughout, so A falling back to 100 returns to the start.
	assert.InDelta(t, 1000, held.FinalAmount, 1e-9)

	options.Rebalance = RebalanceMonthly
	rebalanced, err := Backtrack(context.Background(), prices, evenPie, options)
	require.NoError(t, err)
	// On Feb 3 the $1500 is split evenly again: 3.75 shares of A and 7.5 of B.
	// A then halves: 3.75*100 + 7.5*100.
	assert.InDelta(t, 1125, rebalanced.FinalAmount, 1e-9)
	assert.Equal(t, RebalanceMonthly, rebalanced.Rebalance)

	contributions := 0.0
	for _, slice := range rebalanced.Slices {
		contributions += slice.Contribution
	}
	assert.InDelta(t, 125, contributions, 1e-9, "contributions must still add up to the gain")
}

func TestRebalanceDue(t *testing.T) {
	tests := []struct {
		rebalance Rebalance
		previous  time.Time
		current   time.Time
		want      bool
	}{
		{RebalanceNever, date(2024, 12, 31), date(2025, 1, 2), false},
		{RebalanceMonthly, date(2025, 1, 30), date(2025, 1, 31), false},
		{RebalanceMonthly, date(2025, 1, 31), date(2025, 2, 3), true},
		{RebalanceMonthly, date(2024, 1, 31), date(2025, 1, 2), true},
		{RebalanceQuarterly, date(2025, 1, 31), date(2025, 2, 3), false},
		{RebalanceQuarterly, date(2025, 3, 31), date(2025, 4, 1), true},
		{RebalanceQuarterly, date(2024, 2, 1), date(2025, 2, 3), true},
		{RebalanceYearly, date(2025, 3, 31), date(2025, 4, 1), false},
		{RebalanceYearly, date(2024, 12, 31), date(2025, 1, 2), true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, rebalanceDue(tt.rebalance, tt.previous, tt.current),
			"%s from %s to %s", tt.rebalance, tt.previous.Format(time.DateOnly), tt.current.Format(time.DateOnly))
	}
}

func TestBacktrackStatistics(t *testing.T) {
	days := []time.Time{date(2023, 1, 3), date(2023, 7, 3), date(2024, 1, 3), date(2025, 1, 3)}
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(days, 100, 150, 90, 121),
		"B": history(days, 100, 150, 90, 121),
	}}
	options := Options{From: date(2023, 1, 1), To: date(2025, 1, 31), InitialAmount: 1000}

	result, err := Backtrack(context.Background(), prices, evenPie, options)

	require.NoError(t, err)
	assert.InDelta(t, 21, result.TotalReturnPct, 1e-9)
	// 21% over two years compounds from 10% a year (the period is 731 days, hence the tolerance).
	assert.InDelta(t, 10, result.AnnualizedReturnPct, 0.05)
	// The largest drop is from the 1500 peak to 900.
	assert.InDelta(t, 40, result.MaxDrawdownPct, 1e-9)
}

func TestBacktrackOnlyUsesDaysEverySliceHasAPrice(t *testing.T) {
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(threeDays, 100, 110, 120),
		// B has no price on the middle day and an extra, unordered, duplicated and invalid day.
		"B": {
			{Date: date(2025, 1, 6), Close: 45},
			{Date: date(2025, 1, 2), Close: 50},
			{Date: date(2025, 1, 7), Close: 99},
			{Date: date(2025, 1, 3), Close: 0},
			{Date: time.Date(2025, 1, 6, 14, 30, 0, 0, time.UTC), Close: 45},
		},
	}}

	result, err := Backtrack(context.Background(), prices, evenPie, january)

	require.NoError(t, err)
	assert.Equal(t, []Point{
		{Date: date(2025, 1, 2), Value: 1000},
		{Date: date(2025, 1, 6), Value: 1050},
	}, result.Points)
}

func TestBacktrackIgnoresPricesOutsideThePeriod(t *testing.T) {
	days := []time.Time{date(2024, 12, 31), date(2025, 1, 2), date(2025, 1, 3), date(2025, 2, 3)}
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(days, 1, 100, 110, 9999),
		"B": history(days, 1, 50, 50, 9999),
	}}

	result, err := Backtrack(context.Background(), prices, evenPie, january)

	require.NoError(t, err)
	assert.Equal(t, date(2025, 1, 2), result.From)
	assert.Equal(t, date(2025, 1, 3), result.To)
	assert.InDelta(t, 1050, result.FinalAmount, 1e-9)
}

func TestBacktrackMissingHistory(t *testing.T) {
	year := Options{From: date(2025, 1, 1), To: date(2025, 12, 31), InitialAmount: 1000}
	days := []time.Time{date(2025, 1, 2), date(2025, 6, 2), date(2025, 12, 1)}
	newPrices := func() *fakePrices {
		return &fakePrices{histories: map[string][]investor.DailyPrice{
			"A": history(days, 100, 110, 150),
			"B": history(days, 100, 100, 100),
			// NEW only started trading in June and GONE has no prices at all.
			"NEW": history(days[1:], 10, 30),
		}}
	}
	pie := pieOf(
		investor.Slice{Symbol: "A", Weight: 30},
		investor.Slice{Symbol: "NEW", Weight: 40},
		investor.Slice{Symbol: "B", Weight: 10},
		investor.Slice{Symbol: "GONE", Weight: 20},
	)

	t.Run("fails by default", func(t *testing.T) {
		_, err := Backtrack(context.Background(), newPrices(), pie, year)

		assert.EqualError(t, err, "no price history at the start of the period for: NEW, GONE")
	})

	t.Run("skips and renormalizes when asked", func(t *testing.T) {
		year.SkipMissingHistory = true
		result, err := Backtrack(context.Background(), newPrices(), pie, year)

		require.NoError(t, err)
		assert.Equal(t, []ExcludedSlice{
			{Symbol: "NEW", Weight: 0.4, Reason: "prices only start on 2025-06-02"},
			{Symbol: "GONE", Weight: 0.2, Reason: "no prices in the period"},
		}, result.Excluded)

		// A and B keep their 3:1 proportion, so A is simulated with 75%.
		require.Len(t, result.Slices, 2)
		assert.InDelta(t, 0.75, result.Slices[0].Weight, 1e-9)
		assert.InDelta(t, 0.25, result.Slices[1].Weight, 1e-9)
		assert.InDelta(t, 1375, result.FinalAmount, 1e-9)
		assert.Len(t, result.Points, 3, "NEW's later start must not shorten the period")
	})

	t.Run("fails when nothing is left", func(t *testing.T) {
		prices := &fakePrices{histories: map[string][]investor.DailyPrice{}}
		year.SkipMissingHistory = true

		_, err := Backtrack(context.Background(), prices, evenPie, year)

		assert.EqualError(t, err, "no slice of the pie has price history at the start of the period")
	})
}

func TestBacktrackToleratesAHolidayAtTheStart(t *testing.T) {
	// The period starts on a Saturday before a Monday holiday.
	days := []time.Time{date(2025, 1, 21), date(2025, 1, 22)}
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(days, 100, 110),
		"B": history(days, 100, 100),
	}}
	options := Options{From: date(2025, 1, 18), To: date(2025, 1, 31), InitialAmount: 1000}

	result, err := Backtrack(context.Background(), prices, evenPie, options)

	require.NoError(t, err)
	assert.Equal(t, date(2025, 1, 21), result.From)
}

func TestBacktrackNeedsTwoTradingDays(t *testing.T) {
	prices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(threeDays[:1], 100),
		"B": history(threeDays[:1], 50),
	}}

	_, err := Backtrack(context.Background(), prices, evenPie, january)

	assert.EqualError(t, err, "fewer than two trading days have prices for every slice in the period")
}

func TestBacktrackInvalidInput(t *testing.T) {
	validPrices := &fakePrices{histories: map[string][]investor.DailyPrice{
		"A": history(threeDays, 100, 110, 120),
		"B": history(threeDays, 50, 40, 45),
	}}

	tests := []struct {
		name    string
		pie     investor.Pie
		options Options
		wantErr string
	}{
		{"no period", evenPie, Options{InitialAmount: 1000}, "the period to simulate is not specified"},
		{"period ends before it starts", evenPie, Options{From: date(2025, 2, 1), To: date(2025, 1, 1), InitialAmount: 1000}, "the start of the period must be before its end"},
		{"zero amount", evenPie, Options{From: january.From, To: january.To}, "initial amount ($0.000000) is not valid"},
		{"negative amount", evenPie, Options{From: january.From, To: january.To, InitialAmount: -1}, "is not valid"},
		{"unknown rebalance", evenPie, Options{From: january.From, To: january.To, InitialAmount: 1000, Rebalance: "weekly"}, `rebalance "weekly" is not valid`},
		{"invalid pie", pieOf(investor.Slice{Symbol: "A", Weight: 50}), january, "failed to validate pie: total weight is not 100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Backtrack(context.Background(), validPrices, tt.pie, tt.options)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	assert.Empty(t, validPrices.requested, "no prices may be requested for invalid input")

	_, err := Backtrack(context.Background(), nil, evenPie, january)
	assert.EqualError(t, err, "price source is nil")
}

func TestBacktrackPriceSourceError(t *testing.T) {
	prices := &fakePrices{
		histories: map[string][]investor.DailyPrice{"A": history(threeDays, 100, 110, 120)},
		errs:      map[string]error{"B": errors.New("brokerage down")},
	}

	_, err := Backtrack(context.Background(), prices, evenPie, january)

	assert.EqualError(t, err, "failed to get prices for B: brokerage down")
}

func TestBacktrackRequestsEverySymbolOnce(t *testing.T) {
	slices := []investor.Slice{}
	histories := map[string][]investor.DailyPrice{}
	for _, symbol := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		slices = append(slices, investor.Slice{Symbol: symbol, Weight: 10})
		histories[symbol] = history(threeDays, 100, 110, 120)
	}
	prices := &fakePrices{histories: histories}

	result, err := Backtrack(context.Background(), prices, investor.Pie{Slices: slices}, january)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}, prices.requested)
	assert.InDelta(t, 1200, result.FinalAmount, 1e-9)
}
