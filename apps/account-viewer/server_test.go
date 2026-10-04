package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAccount struct {
	refreshes    int
	refreshErr   error
	positionsErr error
	ordersErr    error
	positions    []investor.Position
	orders       []investor.TradeOrder
	ordersLimit  int

	historyMu       sync.Mutex
	histories       map[string][]investor.DailyPrice
	historyErr      error
	historyRequests int
}

func (f *fakeAccount) RefreshAccount(ctx context.Context) error {
	f.refreshes++
	return f.refreshErr
}

func (f *fakeAccount) DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error) {
	f.historyMu.Lock()
	defer f.historyMu.Unlock()
	f.historyRequests++
	return f.histories[symbol], f.historyErr
}

func (f *fakeAccount) LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	return nil, nil
}

func (f *fakeAccount) TotalCash(ctx context.Context) (float64, error)               { return 1000, nil }
func (f *fakeAccount) CashAvailableForTrading(ctx context.Context) (float64, error) { return 900, nil }
func (f *fakeAccount) CashAvailableForWithdrawal(ctx context.Context) (float64, error) {
	return 800, nil
}
func (f *fakeAccount) LongMarketValue(ctx context.Context) (float64, error)  { return 5000, nil }
func (f *fakeAccount) ShortMarketValue(ctx context.Context) (float64, error) { return 50, nil }
func (f *fakeAccount) PendingDeposits(ctx context.Context) (float64, error)  { return 25, nil }
func (f *fakeAccount) Type(ctx context.Context) (string, error)              { return "MARGIN", nil }

func (f *fakeAccount) Positions(ctx context.Context) ([]investor.Position, error) {
	return f.positions, f.positionsErr
}

func (f *fakeAccount) GetOrderStatus(ctx context.Context, orderID string) (*investor.TradeOrder, error) {
	return nil, nil
}

func (f *fakeAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.TradeOrder, error) {
	f.ordersLimit = limit
	return f.orders, f.ordersErr
}

func request(t *testing.T, account investor.ReadOnlyTradingAccount, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:8090"+target, nil)
	recorder := httptest.NewRecorder()
	newHandler(account, "schwab").ServeHTTP(recorder, req)
	return recorder
}

func TestSnapshot(t *testing.T) {
	filledAt := time.Date(2026, 9, 2, 17, 22, 3, 0, time.UTC)
	account := &fakeAccount{
		positions: []investor.Position{
			{Symbol: "AAPL", Quantity: 3, AveragePrice: 90, CurrentPrice: 100, MarketValue: 300, UnrealizedPL: 30, UnrealizedPLPct: 11.1},
		},
		orders: []investor.TradeOrder{
			{ID: "1", Symbol: "NVDA", Action: investor.OrderActionSell, Type: investor.OrderTypeMarket, Quantity: 10, Status: investor.OrderStatusFilled, FilledQty: 10, FilledPrice: 224.93, SubmittedAt: filledAt, FilledAt: &filledAt, RawResponse: "secret"},
			{ID: "2", Symbol: "ACN", Action: investor.OrderActionSell, Type: investor.OrderTypeMarket, Quantity: 16, Status: investor.OrderStatusPending},
		},
	}

	recorder := request(t, account, http.MethodGet, "/api/snapshot")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.NotContains(t, recorder.Body.String(), "secret", "the brokerage's raw response must not be served")

	var snapshot snapshotJSON
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &snapshot))

	assert.Equal(t, "schwab", snapshot.Brokerage)
	assert.Equal(t, summaryJSON{
		Type:                       "MARGIN",
		TotalCash:                  1000,
		CashAvailableForTrading:    900,
		CashAvailableForWithdrawal: 800,
		LongMarketValue:            5000,
		ShortMarketValue:           50,
		PendingDeposits:            25,
	}, snapshot.Summary)

	require.Len(t, snapshot.Positions, 1)
	assert.Equal(t, "AAPL", snapshot.Positions[0].Symbol)
	assert.Equal(t, 300.0, snapshot.Positions[0].MarketValue)

	require.Len(t, snapshot.Orders, 2)
	assert.Equal(t, 224.93, snapshot.Orders[0].FilledPrice)
	require.NotNil(t, snapshot.Orders[0].SubmittedAt)
	assert.True(t, snapshot.Orders[0].SubmittedAt.Equal(filledAt))
	assert.Nil(t, snapshot.Orders[1].SubmittedAt, "an order without a submission time must not report year 1")
	assert.Nil(t, snapshot.Orders[1].FilledAt)

	assert.Empty(t, snapshot.OrdersError)
	assert.Equal(t, recentOrdersLimit, account.ordersLimit)
}

func TestSnapshotEmptyAccountServesEmptyLists(t *testing.T) {
	recorder := request(t, &fakeAccount{}, http.MethodGet, "/api/snapshot")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"positions":[]`)
	assert.Contains(t, recorder.Body.String(), `"orders":[]`)
}

func TestSnapshotRefreshesAccountEveryTime(t *testing.T) {
	account := &fakeAccount{}
	h := newHandler(account, "schwab")

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/api/snapshot", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, 2, account.refreshes)
}

func TestSnapshotOrdersFailureStillServesTheRest(t *testing.T) {
	account := &fakeAccount{
		positions: []investor.Position{{Symbol: "AAPL", MarketValue: 300}},
		ordersErr: errors.New("orders unavailable"),
	}

	recorder := request(t, account, http.MethodGet, "/api/snapshot")

	require.Equal(t, http.StatusOK, recorder.Code)
	var snapshot snapshotJSON
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &snapshot))
	assert.Equal(t, "orders unavailable", snapshot.OrdersError)
	assert.Len(t, snapshot.Positions, 1)
	assert.Empty(t, snapshot.Orders)
}

func TestSnapshotBrokerageFailure(t *testing.T) {
	tests := []struct {
		name    string
		account *fakeAccount
		wantErr string
	}{
		{"refresh fails", &fakeAccount{refreshErr: errors.New("not authenticated")}, "failed to refresh account: not authenticated"},
		{"positions fail", &fakeAccount{positionsErr: errors.New("brokerage down")}, "failed to get positions: brokerage down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := request(t, tt.account, http.MethodGet, "/api/snapshot")

			assert.Equal(t, http.StatusBadGateway, recorder.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, tt.wantErr, body["error"])
		})
	}
}

func TestOnlyReadMethodsAreAllowed(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, target := range []string{"/api/snapshot", "/", "/api/orders"} {
			account := &fakeAccount{}

			recorder := request(t, account, method, target)

			assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code, "%s %s", method, target)
			assert.Equal(t, 0, account.refreshes, "%s %s must not reach the brokerage", method, target)
		}
	}
}

func TestRequestsForOtherHostsAreRejected(t *testing.T) {
	for _, host := range []string{"evil.example.com", "evil.example.com:8090", "192.168.1.20:8090", "127.0.0.1.evil.example.com"} {
		account := &fakeAccount{}
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/api/snapshot", nil)
		req.Host = host
		recorder := httptest.NewRecorder()

		newHandler(account, "schwab").ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusForbidden, recorder.Code, host)
		assert.Equal(t, 0, account.refreshes, host)
	}
}

func TestLoopbackHostsAreAccepted(t *testing.T) {
	for _, host := range []string{"127.0.0.1:8090", "localhost:8090", "localhost", "[::1]:8090"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/api/snapshot", nil)
		req.Host = host
		recorder := httptest.NewRecorder()

		newHandler(&fakeAccount{}, "schwab").ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code, host)
	}
}

func TestStaticFiles(t *testing.T) {
	for target, contentType := range map[string]string{
		"/":          "text/html",
		"/app.js":    "javascript",
		"/style.css": "text/css",
	} {
		recorder := request(t, &fakeAccount{}, http.MethodGet, target)

		assert.Equal(t, http.StatusOK, recorder.Code, target)
		assert.Contains(t, recorder.Header().Get("Content-Type"), contentType, target)
		assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'self'", target)
	}

	assert.Equal(t, http.StatusNotFound, request(t, &fakeAccount{}, http.MethodGet, "/missing").Code)
}

func TestRequireLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8090", "localhost:0", "[::1]:8090"} {
		assert.NoError(t, requireLoopback(addr), addr)
	}
	for _, addr := range []string{"0.0.0.0:8090", ":8090", "192.168.1.20:8090", "8090"} {
		assert.Error(t, requireLoopback(addr), addr)
	}
}

// recentHistory builds a price history ending today, with one price every week.
func recentHistory(closes ...float64) []investor.DailyPrice {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	prices := make([]investor.DailyPrice, len(closes))
	for i, c := range closes {
		prices[i] = investor.DailyPrice{Date: today.AddDate(0, 0, -7*(len(closes)-1-i)), Close: c}
	}
	return prices
}

// yearOfPrices is a history that starts a year ago with the first price and ends today with the last.
func yearOfPrices(first, last float64) []investor.DailyPrice {
	closes := make([]float64, 53)
	for i := range closes {
		closes[i] = first
	}
	closes[len(closes)-1] = last
	prices := recentHistory(closes...)
	prices[0].Date = time.Now().UTC().Truncate(24*time.Hour).AddDate(-1, 0, 0)
	return prices
}

func backtrackAccount() *fakeAccount {
	return &fakeAccount{
		positions: []investor.Position{
			{Symbol: "AAA", MarketValue: 7500},
			{Symbol: "BBB", MarketValue: 2500},
			{Symbol: "NEW", MarketValue: 10000},
			{Symbol: "SHORT", MarketValue: -500},
		},
		histories: map[string][]investor.DailyPrice{
			"AAA": yearOfPrices(100, 120),
			"BBB": yearOfPrices(50, 50),
			// NEW only has a month of prices.
			"NEW": recentHistory(10, 11, 12, 13),
		},
	}
}

func TestBacktrack(t *testing.T) {
	account := backtrackAccount()

	recorder := request(t, account, http.MethodGet, "/api/backtrack?years=1&rebalance=never")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var result backtrackJSON
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))

	assert.Equal(t, 1, result.Years)
	assert.Equal(t, "never", result.Rebalance)
	assert.Equal(t, 10000.0, result.InitialAmount)

	// NEW has half of the market value but no history, so AAA and BBB are simulated at 75% and 25%.
	require.Len(t, result.Excluded, 1)
	assert.Equal(t, "NEW", result.Excluded[0].Symbol)
	assert.InDelta(t, 0.5, result.Excluded[0].Weight, 1e-9)

	require.Len(t, result.Slices, 2)
	assert.Equal(t, "AAA", result.Slices[0].Symbol)
	assert.InDelta(t, 0.75, result.Slices[0].Weight, 1e-9)
	assert.InDelta(t, 20, result.Slices[0].PriceReturnPct, 1e-9)
	assert.InDelta(t, 1500, result.Slices[0].Contribution, 1e-9)

	// 75% of the amount gains 20%.
	assert.InDelta(t, 11500, result.FinalAmount, 1e-9)
	assert.InDelta(t, 15, result.TotalReturnPct, 1e-9)

	require.NotEmpty(t, result.Points)
	assert.Equal(t, result.From, result.Points[0].Date)
	assert.Equal(t, result.To, result.Points[len(result.Points)-1].Date)
	assert.Equal(t, 10000.0, result.Points[0].Value)

	assert.Equal(t, 3, account.historyRequests, "the short position must not be backtracked")
}

func TestBacktrackDefaults(t *testing.T) {
	recorder := request(t, backtrackAccount(), http.MethodGet, "/api/backtrack")

	require.Equal(t, http.StatusOK, recorder.Code)
	var result backtrackJSON
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Years)
	assert.Equal(t, "never", result.Rebalance)
}

func TestBacktrackReusesPriceHistories(t *testing.T) {
	account := backtrackAccount()
	h := newHandler(account, "schwab")

	for _, rebalance := range []string{"never", "monthly", "yearly"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/api/backtrack?years=1&rebalance="+rebalance, nil)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	assert.Equal(t, 3, account.historyRequests, "each symbol's history must only be requested once")
}

func TestBacktrackInvalidParameters(t *testing.T) {
	for target, wantErr := range map[string]string{
		"/api/backtrack?years=2":          "years must be 1, 3 or 5",
		"/api/backtrack?years=abc":        "years must be 1, 3 or 5",
		"/api/backtrack?rebalance=weekly": "rebalance must be never, monthly, quarterly or yearly",
	} {
		account := backtrackAccount()

		recorder := request(t, account, http.MethodGet, target)

		assert.Equal(t, http.StatusBadRequest, recorder.Code, target)
		assert.Contains(t, recorder.Body.String(), wantErr, target)
		assert.Equal(t, 0, account.historyRequests, target)
	}
}

func TestBacktrackNeedsPositions(t *testing.T) {
	tests := []struct {
		name      string
		positions []investor.Position
		wantErr   string
	}{
		{"no positions", nil, "there are no positions with a market value"},
		{"only a short position", []investor.Position{{Symbol: "SHORT", MarketValue: -500}}, "there are no positions with a market value"},
		{"one position", []investor.Position{{Symbol: "AAA", MarketValue: 100}}, "a pie needs at least two positions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := request(t, &fakeAccount{positions: tt.positions}, http.MethodGet, "/api/backtrack")

			assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tt.wantErr)
		})
	}
}

func TestBacktrackBrokerageFailure(t *testing.T) {
	account := backtrackAccount()
	account.historyErr = errors.New("brokerage down")

	recorder := request(t, account, http.MethodGet, "/api/backtrack")

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "brokerage down")

	recorder = request(t, &fakeAccount{positionsErr: errors.New("no positions today")}, http.MethodGet, "/api/backtrack")

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "failed to get positions: no positions today")
}

func TestBacktrackIsReadOnly(t *testing.T) {
	account := backtrackAccount()

	recorder := request(t, account, http.MethodPost, "/api/backtrack")

	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	assert.Equal(t, 0, account.historyRequests)
}
