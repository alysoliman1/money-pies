package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAccount struct {
	// brokerageCash is what the brokerage holds; totalCash only picks it up on RefreshAccount.
	brokerageCash *float64
	refreshes     int
	refreshErr    error
	totalCash     float64
	positions     []investor.Position
	orders        []investor.TradeOrder
	priceSymbols  []string
	recentLimit   int
	placed        []investor.OrderRequest

	historyMu      sync.Mutex
	histories      map[string][]investor.DailyPrice
	historySymbols []string
	historyFrom    time.Time
	cancelled      []string
	err            error
}

func (f *fakeAccount) RefreshAccount(ctx context.Context) error {
	f.refreshes++
	if f.refreshErr != nil {
		return f.refreshErr
	}
	if f.brokerageCash != nil {
		f.totalCash = *f.brokerageCash
	}
	return nil
}

func (f *fakeAccount) DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error) {
	f.historyMu.Lock()
	defer f.historyMu.Unlock()
	f.historySymbols = append(f.historySymbols, symbol)
	f.historyFrom = from
	return f.histories[symbol], f.err
}

func (f *fakeAccount) LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	f.priceSymbols = symbols
	prices := map[string]float64{}
	for _, s := range symbols {
		prices[s] = 100
	}
	return prices, f.err
}

func (f *fakeAccount) TotalCash(ctx context.Context) (float64, error) { return f.totalCash, f.err }
func (f *fakeAccount) CashAvailableForTrading(ctx context.Context) (float64, error) {
	return 900, f.err
}
func (f *fakeAccount) CashAvailableForWithdrawal(ctx context.Context) (float64, error) {
	return 800, f.err
}
func (f *fakeAccount) LongMarketValue(ctx context.Context) (float64, error)  { return 5000, f.err }
func (f *fakeAccount) ShortMarketValue(ctx context.Context) (float64, error) { return 50, f.err }
func (f *fakeAccount) PendingDeposits(ctx context.Context) (float64, error)  { return 25, f.err }
func (f *fakeAccount) Type(ctx context.Context) (string, error)              { return "MARGIN", f.err }

func (f *fakeAccount) Positions(ctx context.Context) ([]investor.Position, error) {
	return f.positions, f.err
}

func (f *fakeAccount) GetOrderStatus(ctx context.Context, orderID string) (*investor.TradeOrder, error) {
	if f.err != nil {
		return nil, f.err
	}
	filledAt := time.Date(2026, 1, 2, 15, 5, 0, 0, time.UTC)
	return &investor.TradeOrder{
		ID:          orderID,
		Symbol:      "AAPL",
		Action:      investor.OrderActionBuy,
		Type:        investor.OrderTypeMarket,
		Quantity:    2,
		Status:      investor.OrderStatusFilled,
		FilledQty:   2,
		FilledPrice: 101.5,
		FilledAt:    &filledAt,
	}, nil
}

func (f *fakeAccount) PlaceOrder(ctx context.Context, order investor.OrderRequest) (*investor.TradeOrder, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.placed = append(f.placed, order)
	return &investor.TradeOrder{
		ID:          "order-1",
		Symbol:      order.Symbol,
		Action:      order.Action,
		Type:        order.Type,
		Quantity:    order.Quantity,
		LimitPrice:  order.LimitPrice,
		Status:      investor.OrderStatusPending,
		SubmittedAt: time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		RawResponse: map[string]any{"secret": "raw"},
	}, nil
}

func (f *fakeAccount) CancelPendingOrder(ctx context.Context, orderID string) error {
	if f.err != nil {
		return f.err
	}
	f.cancelled = append(f.cancelled, orderID)
	return nil
}

func (f *fakeAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.TradeOrder, error) {
	f.recentLimit = limit
	return f.orders, f.err
}

// connect starts the server over an in-memory transport and returns a connected client session.
func connect(t *testing.T, account investor.TradingAccount) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := New(account).Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { clientSession.Close() })

	return clientSession
}

// callTool calls a tool and decodes its structured output into out (if non-nil).
func callTool(t *testing.T, session *mcp.ClientSession, name string, args any, out any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)

	if out != nil && !result.IsError {
		raw, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, out))
	}
	return result
}

func errorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.True(t, result.IsError, "expected a tool error")
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestListTools(t *testing.T) {
	session := connect(t, &fakeAccount{totalCash: 1000})

	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)

	readOnly := map[string]bool{}
	for _, tool := range result.Tools {
		readOnly[tool.Name] = tool.Annotations.ReadOnlyHint
	}
	assert.Equal(t, map[string]bool{
		"get_account_summary": true,
		"get_positions":       true,
		"get_latest_prices":   true,
		"get_order_status":    true,
		"get_recent_orders":   true,
		"backtrack":           true,
		"place_order":         false,
		"cancel_order":        false,
	}, readOnly)
}

func TestAccountSummary(t *testing.T) {
	session := connect(t, &fakeAccount{totalCash: 1000})

	var summary AccountSummary
	callTool(t, session, "get_account_summary", map[string]any{}, &summary)

	assert.Equal(t, AccountSummary{
		Type:                       "MARGIN",
		TotalCash:                  1000,
		CashAvailableForTrading:    900,
		CashAvailableForWithdrawal: 800,
		LongMarketValue:            5000,
		ShortMarketValue:           50,
		PendingDeposits:            25,
	}, summary)
}

func TestPositions(t *testing.T) {
	account := &fakeAccount{positions: []investor.Position{
		{Symbol: "AAPL", Quantity: 3, AveragePrice: 90, CurrentPrice: 100, MarketValue: 300, UnrealizedPL: 30, UnrealizedPLPct: 11.1},
	}}
	session := connect(t, account)

	var out PositionsOutput
	callTool(t, session, "get_positions", map[string]any{}, &out)

	require.Len(t, out.Positions, 1)
	assert.Equal(t, "AAPL", out.Positions[0].Symbol)
	assert.Equal(t, 300.0, out.Positions[0].MarketValue)
}

func TestPositionsEmpty(t *testing.T) {
	session := connect(t, &fakeAccount{totalCash: 1000})

	var out PositionsOutput
	result := callTool(t, session, "get_positions", map[string]any{}, &out)

	assert.False(t, result.IsError)
	assert.Empty(t, out.Positions)
}

func TestLatestPrices(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	var out LatestPricesOutput
	callTool(t, session, "get_latest_prices", map[string]any{"symbols": []string{" aapl ", "MSFT", ""}}, &out)

	assert.Equal(t, []string{"AAPL", "MSFT"}, account.priceSymbols)
	assert.Equal(t, map[string]float64{"AAPL": 100, "MSFT": 100}, out.Prices)
}

func TestLatestPricesRequiresSymbols(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	result := callTool(t, session, "get_latest_prices", map[string]any{"symbols": []string{}}, nil)

	assert.Contains(t, errorText(t, result), "at least one symbol")
	assert.Nil(t, account.priceSymbols)
}

func TestOrderStatus(t *testing.T) {
	session := connect(t, &fakeAccount{totalCash: 1000})

	var order Order
	callTool(t, session, "get_order_status", map[string]any{"order_id": "abc"}, &order)

	assert.Equal(t, "abc", order.ID)
	assert.Equal(t, "FILLED", order.Status)
	assert.Equal(t, 101.5, order.FilledPrice)
	assert.Equal(t, "2026-01-02T15:05:00Z", order.FilledAt)
	assert.Empty(t, order.SubmittedAt)
}

func TestRecentOrders(t *testing.T) {
	account := &fakeAccount{orders: []investor.TradeOrder{
		{ID: "1", Symbol: "AAPL", Action: investor.OrderActionBuy, Type: investor.OrderTypeMarket, Status: investor.OrderStatusFilled},
		{ID: "2", Symbol: "MSFT", Action: investor.OrderActionSell, Type: investor.OrderTypeMarket, Status: investor.OrderStatusCancelled},
	}}
	session := connect(t, account)

	var out RecentOrdersOutput
	callTool(t, session, "get_recent_orders", map[string]any{}, &out)
	require.Len(t, out.Orders, 2)
	assert.Equal(t, "2", out.Orders[1].ID)
	assert.Equal(t, defaultRecentOrdersLimit, account.recentLimit)

	callTool(t, session, "get_recent_orders", map[string]any{"limit": 5}, &out)
	assert.Equal(t, 5, account.recentLimit)
}

func TestRecentOrdersLimitBounds(t *testing.T) {
	session := connect(t, &fakeAccount{totalCash: 1000})

	for _, limit := range []int{-1, maxRecentOrdersLimit + 1} {
		result := callTool(t, session, "get_recent_orders", map[string]any{"limit": limit}, nil)
		assert.Contains(t, errorText(t, result), "limit must be between")
	}
}

func TestBrokerageErrorIsToolError(t *testing.T) {
	session := connect(t, &fakeAccount{err: errors.New("brokerage down")})

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"get_account_summary", map[string]any{}},
		{"get_positions", map[string]any{}},
		{"get_latest_prices", map[string]any{"symbols": []string{"AAPL"}}},
		{"get_order_status", map[string]any{"order_id": "abc"}},
		{"get_recent_orders", map[string]any{}},
		{"backtrack", map[string]any{"slices": []map[string]any{{"symbol": "AAA", "weight": 1}, {"symbol": "BBB", "weight": 1}}}},
		{"place_order", map[string]any{"symbol": "AAPL", "action": "BUY", "type": "MARKET", "quantity": 1}},
		{"cancel_order", map[string]any{"order_id": "abc"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			result := callTool(t, session, tc.tool, tc.args, nil)
			assert.Contains(t, errorText(t, result), "brokerage down")
		})
	}
}

func TestOrderStatusRequiresOrderID(t *testing.T) {
	session := connect(t, &fakeAccount{})

	result := callTool(t, session, "get_order_status", map[string]any{"order_id": "  "}, nil)

	assert.Contains(t, errorText(t, result), "order_id is required")
}

// Brokerage accounts only load their balances on RefreshAccount, so the summary
// must refresh the account on every call instead of reading what was loaded before.
func TestBalancesAreNotStale(t *testing.T) {
	brokerageCash := 1000.0
	account := &fakeAccount{brokerageCash: &brokerageCash}
	session := connect(t, account)

	var summary AccountSummary
	callTool(t, session, "get_account_summary", map[string]any{}, &summary)
	assert.Equal(t, 1000.0, summary.TotalCash)

	brokerageCash = 250
	callTool(t, session, "get_account_summary", map[string]any{}, &summary)
	assert.Equal(t, 250.0, summary.TotalCash)
	assert.Equal(t, 2, account.refreshes)
}

func TestRefreshErrorIsToolError(t *testing.T) {
	session := connect(t, &fakeAccount{refreshErr: errors.New("not authenticated")})

	result := callTool(t, session, "get_account_summary", map[string]any{}, nil)

	assert.Contains(t, errorText(t, result), "failed to refresh account: not authenticated")
}

// Only the balances are loaded by RefreshAccount; the other tools read from the brokerage directly.
func TestOnlySummaryRefreshesAccount(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	callTool(t, session, "get_positions", map[string]any{}, nil)
	callTool(t, session, "get_latest_prices", map[string]any{"symbols": []string{"AAPL"}}, nil)
	callTool(t, session, "get_order_status", map[string]any{"order_id": "abc"}, nil)
	callTool(t, session, "get_recent_orders", map[string]any{}, nil)

	assert.Equal(t, 0, account.refreshes)
}

func TestPlaceOrder(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	var order Order
	result := callTool(t, session, "place_order", map[string]any{
		"symbol":      "aapl",
		"action":      "sell",
		"type":        "limit",
		"quantity":    2.5,
		"limit_price": 99.5,
	}, &order)

	require.Len(t, account.placed, 1)
	placed := account.placed[0]
	assert.Equal(t, "AAPL", placed.Symbol)
	assert.Equal(t, investor.OrderActionSell, placed.Action)
	assert.Equal(t, investor.OrderTypeLimit, placed.Type)
	assert.Equal(t, 2.5, placed.Quantity)
	require.NotNil(t, placed.LimitPrice)
	assert.Equal(t, 99.5, *placed.LimitPrice)

	assert.Equal(t, "order-1", order.ID)
	assert.Equal(t, "PENDING", order.Status)
	assert.Equal(t, "2026-01-02T15:04:05Z", order.SubmittedAt)
	assert.Empty(t, order.FilledAt)

	// The brokerage's raw response must not leak into the tool output.
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
}

func TestPlaceMarketOrder(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	callTool(t, session, "place_order", map[string]any{"symbol": "MSFT", "action": "BUY", "type": "MARKET", "quantity": 3}, nil)

	assert.Equal(t, []investor.OrderRequest{
		{Symbol: "MSFT", Action: investor.OrderActionBuy, Type: investor.OrderTypeMarket, Quantity: 3},
	}, account.placed)
}

func TestPlaceOrderValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name:    "blank symbol",
			args:    map[string]any{"symbol": " ", "action": "BUY", "type": "MARKET", "quantity": 1},
			wantErr: "symbol is required",
		},
		{
			name:    "unknown action",
			args:    map[string]any{"symbol": "AAPL", "action": "HOLD", "type": "MARKET", "quantity": 1},
			wantErr: "action must be BUY or SELL",
		},
		{
			name:    "unknown type",
			args:    map[string]any{"symbol": "AAPL", "action": "BUY", "type": "STOP", "quantity": 1},
			wantErr: "type must be MARKET or LIMIT",
		},
		{
			name:    "zero quantity",
			args:    map[string]any{"symbol": "AAPL", "action": "BUY", "type": "MARKET", "quantity": 0},
			wantErr: "quantity must be greater than zero",
		},
		{
			name:    "negative quantity",
			args:    map[string]any{"symbol": "AAPL", "action": "SELL", "type": "MARKET", "quantity": -1},
			wantErr: "quantity must be greater than zero",
		},
		{
			name:    "limit order without price",
			args:    map[string]any{"symbol": "AAPL", "action": "BUY", "type": "LIMIT", "quantity": 1},
			wantErr: "limit_price must be greater than zero",
		},
		{
			name:    "limit order with zero price",
			args:    map[string]any{"symbol": "AAPL", "action": "BUY", "type": "LIMIT", "quantity": 1, "limit_price": 0},
			wantErr: "limit_price must be greater than zero",
		},
		{
			name:    "market order with limit price",
			args:    map[string]any{"symbol": "AAPL", "action": "BUY", "type": "MARKET", "quantity": 1, "limit_price": 10},
			wantErr: "limit_price is not allowed for MARKET orders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &fakeAccount{}
			session := connect(t, account)

			result := callTool(t, session, "place_order", tt.args, nil)

			assert.Contains(t, errorText(t, result), tt.wantErr)
			assert.Empty(t, account.placed, "invalid order must not reach the brokerage")
		})
	}
}

func TestPlaceOrderMissingRequiredField(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	// The SDK rejects this against the input schema; depending on the SDK version
	// that is either a protocol error or a tool error.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "place_order",
		Arguments: map[string]any{"symbol": "AAPL", "action": "BUY", "type": "MARKET"},
	})

	if err == nil {
		assert.True(t, result.IsError)
	}
	assert.Empty(t, account.placed)
}

func TestCancelOrder(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	var out CancelOrderOutput
	callTool(t, session, "cancel_order", map[string]any{"order_id": " abc "}, &out)

	assert.Equal(t, CancelOrderOutput{OrderID: "abc", Cancelled: true}, out)
	assert.Equal(t, []string{"abc"}, account.cancelled)
}

func TestCancelOrderRequiresOrderID(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, account)

	result := callTool(t, session, "cancel_order", map[string]any{"order_id": "  "}, nil)

	assert.Contains(t, errorText(t, result), "order_id is required")
	assert.Empty(t, account.cancelled)
}

// yearOfPrices builds a history of weekly prices that starts a year ago with the first
// price and ends today with the last price.
func yearOfPrices(first, last float64) []investor.DailyPrice {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	prices := []investor.DailyPrice{{Date: today.AddDate(-1, 0, 0), Close: first}}
	for week := 51; week >= 1; week-- {
		prices = append(prices, investor.DailyPrice{Date: today.AddDate(0, 0, -7*week), Close: first})
	}
	return append(prices, investor.DailyPrice{Date: today, Close: last})
}

func backtrackAccount() *fakeAccount {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return &fakeAccount{
		positions: []investor.Position{
			{Symbol: "AAA", MarketValue: 7500},
			{Symbol: "BBB", MarketValue: 2500},
		},
		histories: map[string][]investor.DailyPrice{
			"AAA": yearOfPrices(100, 120),
			"BBB": yearOfPrices(50, 50),
			"CCC": yearOfPrices(10, 20),
			// NEW only started trading a week ago.
			"NEW": {{Date: today.AddDate(0, 0, -7), Close: 10}, {Date: today, Close: 11}},
		},
	}
}

func TestBacktrackCurrentPositions(t *testing.T) {
	account := backtrackAccount()
	session := connect(t, account)

	var out BacktrackOutput
	callTool(t, session, "backtrack", map[string]any{}, &out)

	assert.Equal(t, "current_positions", out.Source)
	assert.Equal(t, "never", out.Rebalance)
	assert.Equal(t, 10000.0, out.InitialAmount)
	// 75% of the amount gains 20%.
	assert.InDelta(t, 11500, out.FinalAmount, 1e-9)
	assert.InDelta(t, 15, out.TotalReturnPct, 1e-9)
	assert.Empty(t, out.Excluded)

	require.Len(t, out.Slices, 2)
	assert.Equal(t, "AAA", out.Slices[0].Symbol)
	assert.InDelta(t, 75, out.Slices[0].WeightPct, 1e-9)
	assert.InDelta(t, 20, out.Slices[0].PriceReturnPct, 1e-9)
	assert.InDelta(t, 1500, out.Slices[0].Contribution, 1e-9)

	assert.ElementsMatch(t, []string{"AAA", "BBB"}, account.historySymbols)
	assert.WithinDuration(t, time.Now().AddDate(-1, 0, 0), account.historyFrom, time.Minute)
}

func TestBacktrackMonthEndValues(t *testing.T) {
	session := connect(t, backtrackAccount())

	var out BacktrackOutput
	callTool(t, session, "backtrack", map[string]any{}, &out)

	// A year of weekly prices spans 12 or 13 calendar months, plus the first day.
	require.GreaterOrEqual(t, len(out.MonthEndValues), 13)
	require.LessOrEqual(t, len(out.MonthEndValues), 15)
	assert.Equal(t, out.From, out.MonthEndValues[0].Date)
	assert.Equal(t, 10000.0, out.MonthEndValues[0].Value)
	assert.Equal(t, out.To, out.MonthEndValues[len(out.MonthEndValues)-1].Date)
	assert.InDelta(t, out.FinalAmount, out.MonthEndValues[len(out.MonthEndValues)-1].Value, 1e-9)

	months := map[string]int{}
	for _, point := range out.MonthEndValues[1:] {
		months[point.Date[:7]]++
	}
	for month, count := range months {
		// The first day's month may also appear as that month's end.
		assert.LessOrEqual(t, count, 1, month)
	}
}

func TestBacktrackGivenSlices(t *testing.T) {
	account := backtrackAccount()
	session := connect(t, account)

	var out BacktrackOutput
	callTool(t, session, "backtrack", map[string]any{
		"slices": []map[string]any{
			{"symbol": " ccc ", "weight": 3},
			{"symbol": "BBB", "weight": 1},
			{"symbol": "NEW", "weight": 4},
		},
		"years":          1,
		"rebalance":      "Monthly",
		"initial_amount": 2000,
	}, &out)

	assert.Equal(t, "given_slices", out.Source)
	assert.Equal(t, "monthly", out.Rebalance)
	assert.Equal(t, 2000.0, out.InitialAmount)

	// NEW has half of the weight but no history, so CCC and BBB keep their 3:1 proportion.
	require.Len(t, out.Excluded, 1)
	assert.Equal(t, "NEW", out.Excluded[0].Symbol)
	assert.InDelta(t, 50, out.Excluded[0].WeightPct, 1e-9)

	require.Len(t, out.Slices, 2)
	assert.Equal(t, "CCC", out.Slices[0].Symbol)
	assert.InDelta(t, 75, out.Slices[0].WeightPct, 1e-9)
	// CCC doubles on the last day: 75% of $2000 becomes $3000.
	assert.InDelta(t, 3500, out.FinalAmount, 1e-9)

	assert.NotContains(t, account.historySymbols, "AAA", "the account's positions must not be used when slices are given")
}

func TestBacktrackInvalidInput(t *testing.T) {
	twoSlices := []map[string]any{{"symbol": "AAA", "weight": 1}, {"symbol": "BBB", "weight": 1}}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"years too small", map[string]any{"years": -1}, "years must be between 1 and 20"},
		{"years too large", map[string]any{"years": 21}, "years must be between 1 and 20"},
		{"unknown rebalance", map[string]any{"rebalance": "weekly"}, "rebalance must be never, monthly, quarterly or yearly"},
		{"negative amount", map[string]any{"initial_amount": -5}, "initial_amount must be greater than zero"},
		{"one slice", map[string]any{"slices": twoSlices[:1]}, "slices must hold at least two symbols"},
		{"blank symbol", map[string]any{"slices": []map[string]any{{"symbol": " ", "weight": 1}, {"symbol": "BBB", "weight": 1}}}, "every slice needs a symbol"},
		{"zero weight", map[string]any{"slices": []map[string]any{{"symbol": "AAA", "weight": 0}, {"symbol": "BBB", "weight": 1}}}, "weight for AAA must be greater than zero"},
		{"duplicate symbol", map[string]any{"slices": []map[string]any{{"symbol": "AAA", "weight": 1}, {"symbol": "aaa", "weight": 1}}}, "symbol AAA is duplicated in the pie"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := backtrackAccount()
			session := connect(t, account)

			result := callTool(t, session, "backtrack", tt.args, nil)

			assert.Contains(t, errorText(t, result), tt.wantErr)
			assert.Empty(t, account.historySymbols, "no prices may be requested for invalid input")
		})
	}
}

func TestBacktrackNeedsTwoPositions(t *testing.T) {
	session := connect(t, &fakeAccount{positions: []investor.Position{{Symbol: "AAA", MarketValue: 100}}})

	result := callTool(t, session, "backtrack", map[string]any{}, nil)

	assert.Contains(t, errorText(t, result), "a pie needs at least two positions")
}
