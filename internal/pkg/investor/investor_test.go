package investor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAccount implements the TradingAccount methods used by the Investor.
// Calling any other method panics through the nil embedded interface.
type fakeAccount struct {
	TradingAccount
	// brokerageCash is what the brokerage holds; cash only picks it up on RefreshAccount.
	brokerageCash *float64
	refreshErr    error
	cash          float64
	cashErr       error
	prices        map[string]float64
	pricesErr     error
	failSymbols   map[string]bool
	pricesSymbols []string
	placed        []OrderRequest
}

func (f *fakeAccount) RefreshAccount(ctx context.Context) error {
	if f.refreshErr != nil {
		return f.refreshErr
	}
	if f.brokerageCash != nil {
		f.cash = *f.brokerageCash
	}
	return nil
}

func (f *fakeAccount) CashAvailableForTrading(ctx context.Context) (float64, error) {
	return f.cash, f.cashErr
}

func (f *fakeAccount) LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	f.pricesSymbols = symbols
	return f.prices, f.pricesErr
}

func (f *fakeAccount) PlaceOrder(ctx context.Context, order OrderRequest) (*TradeOrder, error) {
	if f.failSymbols[order.Symbol] {
		return nil, errors.New("order rejected")
	}
	f.placed = append(f.placed, order)
	return &TradeOrder{ID: order.Symbol, Status: OrderStatusPending}, nil
}

func pieOf(slices ...Slice) Pie {
	return Pie{Slices: slices}
}

func TestWholeShareQuantities(t *testing.T) {
	tests := []struct {
		name        string
		amount      float64
		symbols     []string
		weights     map[string]float64
		prices      map[string]float64
		preInvested map[string]float64
		want        map[string]float64
	}{
		{
			name:    "amounts divide evenly into shares",
			amount:  1000,
			symbols: []string{"A", "B"},
			weights: map[string]float64{"A": 0.5, "B": 0.5},
			prices:  map[string]float64{"A": 100, "B": 50},
			want:    map[string]float64{"A": 5, "B": 10},
		},
		{
			name:    "left over too small for any share",
			amount:  1000,
			symbols: []string{"A", "B"},
			weights: map[string]float64{"A": 0.5, "B": 0.5},
			prices:  map[string]float64{"A": 100, "B": 30},
			want:    map[string]float64{"A": 5, "B": 16},
		},
		{
			name:    "left over buys an extra share of the only affordable symbol",
			amount:  1000,
			symbols: []string{"A", "B"},
			weights: map[string]float64{"A": 0.6, "B": 0.4},
			prices:  map[string]float64{"A": 250, "B": 90},
			want:    map[string]float64{"A": 2, "B": 5},
		},
		{
			name:        "pre invested amount reduces what is bought",
			amount:      1000,
			symbols:     []string{"A", "B"},
			weights:     map[string]float64{"A": 0.5, "B": 0.5},
			prices:      map[string]float64{"A": 100, "B": 50},
			preInvested: map[string]float64{"A": 300},
			// A needs $200 (2 shares) and B $500 (10 shares). The $300 left over goes
			// to whichever symbol ends up the least above its $500 target.
			want: map[string]float64{"A": 3, "B": 14},
		},
		{
			name:        "pre invested amount above the target",
			amount:      1000,
			symbols:     []string{"A", "B"},
			weights:     map[string]float64{"A": 0.5, "B": 0.5},
			prices:      map[string]float64{"A": 100, "B": 50},
			preInvested: map[string]float64{"A": 500},
			want:        map[string]float64{"A": 2, "B": 16},
		},
		{
			name:    "share price above the whole amount",
			amount:  100,
			symbols: []string{"A", "B"},
			weights: map[string]float64{"A": 0.5, "B": 0.5},
			prices:  map[string]float64{"A": 500, "B": 20},
			want:    map[string]float64{"A": 0, "B": 5},
		},
		{
			name:    "prices that are not exact in floating point",
			amount:  100,
			symbols: []string{"A", "B"},
			weights: map[string]float64{"A": 0.3, "B": 0.7},
			prices:  map[string]float64{"A": 0.1, "B": 0.7},
			want:    map[string]float64{"A": 300, "B": 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wholeShareQuantities(tt.amount, tt.symbols, tt.weights, tt.prices, tt.preInvested)
			assert.Equal(t, tt.want, got)

			spent := 0.0
			cheapest := tt.prices[tt.symbols[0]]
			for _, symbol := range tt.symbols {
				spent += got[symbol] * tt.prices[symbol]
				cheapest = min(cheapest, tt.prices[symbol])

				needed := max(tt.amount*tt.weights[symbol]-tt.preInvested[symbol], 0)
				assert.GreaterOrEqual(t, (got[symbol]+1)*tt.prices[symbol], needed-floatTolerance,
					"%s is more than one share short of its target", symbol)
			}
			assert.LessOrEqual(t, spent, tt.amount+floatTolerance, "spent more than the amount")
			assert.Less(t, tt.amount-spent, cheapest, "a share was still affordable")
		})
	}
}

func TestPlacePieOrderWithoutFractionalShares(t *testing.T) {
	account := &fakeAccount{
		cash:   5000,
		prices: map[string]float64{"AAPL": 250, "NVDA": 90, "BRK.A": 700000},
	}
	pie := pieOf(Slice{Symbol: "AAPL", Weight: 50}, Slice{Symbol: "BRK.A", Weight: 10}, Slice{Symbol: "NVDA", Weight: 40})

	err := NewInvestor(account).PlacePieOrderWithoutFractionalShares(context.Background(), 1000, pie, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"AAPL", "BRK.A", "NVDA"}, account.pricesSymbols)
	// BRK.A is too expensive for any share, so no order is placed for it.
	assert.Equal(t, []OrderRequest{
		{Symbol: "AAPL", Action: OrderActionBuy, Type: OrderTypeMarket, Quantity: 2},
		{Symbol: "NVDA", Action: OrderActionBuy, Type: OrderTypeMarket, Quantity: 5},
	}, account.placed)
}

func TestPlacePieOrderWithoutFractionalSharesInvalidInput(t *testing.T) {
	validPie := pieOf(Slice{Symbol: "AAPL", Weight: 50}, Slice{Symbol: "NVDA", Weight: 50})
	validPrices := map[string]float64{"AAPL": 100, "NVDA": 50}

	tests := []struct {
		name        string
		account     *fakeAccount
		amount      float64
		pie         Pie
		preInvested map[string]float64
		wantErr     string
	}{
		{
			name:    "zero amount",
			account: &fakeAccount{cash: 5000, prices: validPrices},
			amount:  0,
			pie:     validPie,
			wantErr: "amount to invest ($0.000000) is not valid",
		},
		{
			name:    "negative amount",
			account: &fakeAccount{cash: 5000, prices: validPrices},
			amount:  -10,
			pie:     validPie,
			wantErr: "is not valid",
		},
		{
			name:    "amount above cash available for trading",
			account: &fakeAccount{cash: 500, prices: validPrices},
			amount:  1000,
			pie:     validPie,
			wantErr: "is greater than cash available for trading",
		},
		{
			name:    "account refresh fails",
			account: &fakeAccount{cash: 5000, refreshErr: errors.New("brokerage down"), prices: validPrices},
			amount:  1000,
			pie:     validPie,
			wantErr: "failed to refresh account: brokerage down",
		},
		{
			name:    "cash lookup fails",
			account: &fakeAccount{cashErr: errors.New("brokerage down"), prices: validPrices},
			amount:  1000,
			pie:     validPie,
			wantErr: "failed to get cash available for trading: brokerage down",
		},
		{
			name:    "invalid pie",
			account: &fakeAccount{cash: 5000, prices: validPrices},
			amount:  1000,
			pie:     pieOf(Slice{Symbol: "AAPL", Weight: 50}),
			wantErr: "failed to validate pie: total weight is not 100",
		},
		{
			name:        "pre invested symbol not in the pie",
			account:     &fakeAccount{cash: 5000, prices: validPrices},
			amount:      1000,
			pie:         validPie,
			preInvested: map[string]float64{"MSFT": 100},
			wantErr:     "pre invested symbol MSFT is not in the pie",
		},
		{
			name:        "negative pre invested amount",
			account:     &fakeAccount{cash: 5000, prices: validPrices},
			amount:      1000,
			pie:         validPie,
			preInvested: map[string]float64{"AAPL": -1},
			wantErr:     "pre invested amount for symbol AAPL",
		},
		{
			name:    "price lookup fails",
			account: &fakeAccount{cash: 5000, pricesErr: errors.New("brokerage down")},
			amount:  1000,
			pie:     validPie,
			wantErr: "brokerage down",
		},
		{
			name:    "price missing for a symbol",
			account: &fakeAccount{cash: 5000, prices: map[string]float64{"AAPL": 100}},
			amount:  1000,
			pie:     validPie,
			wantErr: "price for symbol NVDA ($0.000000) is not valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewInvestor(tt.account).PlacePieOrderWithoutFractionalShares(context.Background(), tt.amount, tt.pie, tt.preInvested)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, tt.account.placed, "no order may be placed on invalid input")
		})
	}
}

func TestPlacePieOrderWithoutFractionalSharesNoAccount(t *testing.T) {
	pie := pieOf(Slice{Symbol: "AAPL", Weight: 50}, Slice{Symbol: "NVDA", Weight: 50})

	err := NewInvestor(nil).PlacePieOrderWithoutFractionalShares(context.Background(), 1000, pie, nil)

	assert.EqualError(t, err, "investor has no trading account")
}

func TestPlacePieOrderWithoutFractionalSharesOrderFailure(t *testing.T) {
	account := &fakeAccount{
		cash:        5000,
		prices:      map[string]float64{"AAPL": 100, "NVDA": 50, "MSFT": 25},
		failSymbols: map[string]bool{"AAPL": true},
	}
	pie := pieOf(Slice{Symbol: "AAPL", Weight: 50}, Slice{Symbol: "NVDA", Weight: 25}, Slice{Symbol: "MSFT", Weight: 25})

	err := NewInvestor(account).PlacePieOrderWithoutFractionalShares(context.Background(), 1000, pie, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to place order for 5 shares of AAPL: order rejected")
	// The other orders are still placed.
	assert.Equal(t, []OrderRequest{
		{Symbol: "NVDA", Action: OrderActionBuy, Type: OrderTypeMarket, Quantity: 5},
		{Symbol: "MSFT", Action: OrderActionBuy, Type: OrderTypeMarket, Quantity: 10},
	}, account.placed)
}

// The cash check must use the balance at the brokerage, not the one loaded when the account was created.
func TestPlacePieOrderWithoutFractionalSharesRefreshesCash(t *testing.T) {
	pie := pieOf(Slice{Symbol: "AAPL", Weight: 50}, Slice{Symbol: "NVDA", Weight: 50})
	prices := map[string]float64{"AAPL": 100, "NVDA": 50}

	brokerageCash := 500.0
	stale := &fakeAccount{cash: 5000, brokerageCash: &brokerageCash, prices: prices}
	err := NewInvestor(stale).PlacePieOrderWithoutFractionalShares(context.Background(), 1000, pie, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is greater than cash available for trading ($500.000000)")
	assert.Empty(t, stale.placed)

	brokerageCash = 5000
	toppedUp := &fakeAccount{cash: 500, brokerageCash: &brokerageCash, prices: prices}
	err = NewInvestor(toppedUp).PlacePieOrderWithoutFractionalShares(context.Background(), 1000, pie, nil)
	require.NoError(t, err)
	assert.Len(t, toppedUp.placed, 2)
}
