package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAccount struct {
	totalCash    float64
	positions    []investor.Position
	orders       []investor.TradeOrder
	priceSymbols []string
	recentLimit  int
	err          error
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

func (f *fakeAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.TradeOrder, error) {
	f.recentLimit = limit
	return f.orders, f.err
}

// staticAccount returns an AccountProvider that always returns the given account.
func staticAccount(account ReadOnlyAccount) AccountProvider {
	return func(ctx context.Context) (ReadOnlyAccount, error) {
		return account, nil
	}
}

// connect starts the server over an in-memory transport and returns a connected client session.
func connect(t *testing.T, provider AccountProvider) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := New(provider).Connect(ctx, serverTransport, nil)
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
	session := connect(t, staticAccount(&fakeAccount{totalCash: 1000}))

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
	}, readOnly)
}

func TestAccountSummary(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{totalCash: 1000}))

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
	session := connect(t, staticAccount(account))

	var out PositionsOutput
	callTool(t, session, "get_positions", map[string]any{}, &out)

	require.Len(t, out.Positions, 1)
	assert.Equal(t, "AAPL", out.Positions[0].Symbol)
	assert.Equal(t, 300.0, out.Positions[0].MarketValue)
}

func TestPositionsEmpty(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{totalCash: 1000}))

	var out PositionsOutput
	result := callTool(t, session, "get_positions", map[string]any{}, &out)

	assert.False(t, result.IsError)
	assert.Empty(t, out.Positions)
}

func TestLatestPrices(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, staticAccount(account))

	var out LatestPricesOutput
	callTool(t, session, "get_latest_prices", map[string]any{"symbols": []string{" aapl ", "MSFT", ""}}, &out)

	assert.Equal(t, []string{"AAPL", "MSFT"}, account.priceSymbols)
	assert.Equal(t, map[string]float64{"AAPL": 100, "MSFT": 100}, out.Prices)
}

func TestLatestPricesRequiresSymbols(t *testing.T) {
	account := &fakeAccount{}
	session := connect(t, staticAccount(account))

	result := callTool(t, session, "get_latest_prices", map[string]any{"symbols": []string{}}, nil)

	assert.Contains(t, errorText(t, result), "at least one symbol")
	assert.Nil(t, account.priceSymbols)
}

func TestOrderStatus(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{totalCash: 1000}))

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
	session := connect(t, staticAccount(account))

	var out RecentOrdersOutput
	callTool(t, session, "get_recent_orders", map[string]any{}, &out)
	require.Len(t, out.Orders, 2)
	assert.Equal(t, "2", out.Orders[1].ID)
	assert.Equal(t, defaultRecentOrdersLimit, account.recentLimit)

	callTool(t, session, "get_recent_orders", map[string]any{"limit": 5}, &out)
	assert.Equal(t, 5, account.recentLimit)
}

func TestRecentOrdersLimitBounds(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{totalCash: 1000}))

	for _, limit := range []int{-1, maxRecentOrdersLimit + 1} {
		result := callTool(t, session, "get_recent_orders", map[string]any{"limit": limit}, nil)
		assert.Contains(t, errorText(t, result), "limit must be between")
	}
}

func TestBrokerageErrorIsToolError(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{err: errors.New("brokerage down")}))

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"get_account_summary", map[string]any{}},
		{"get_positions", map[string]any{}},
		{"get_latest_prices", map[string]any{"symbols": []string{"AAPL"}}},
		{"get_order_status", map[string]any{"order_id": "abc"}},
		{"get_recent_orders", map[string]any{}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			result := callTool(t, session, tc.tool, tc.args, nil)
			assert.Contains(t, errorText(t, result), "brokerage down")
		})
	}
}

func TestProviderErrorIsToolError(t *testing.T) {
	session := connect(t, func(ctx context.Context) (ReadOnlyAccount, error) {
		return nil, errors.New("not authenticated")
	})

	result := callTool(t, session, "get_positions", map[string]any{}, nil)

	assert.Contains(t, errorText(t, result), "not authenticated")
}

func TestOrderStatusRequiresOrderID(t *testing.T) {
	session := connect(t, staticAccount(&fakeAccount{}))

	result := callTool(t, session, "get_order_status", map[string]any{"order_id": "  "}, nil)

	assert.Contains(t, errorText(t, result), "order_id is required")
}

// Brokerage accounts snapshot their balances at construction, so every tool call
// must load a new account instead of reusing one from an earlier call.
func TestBalancesAreNotStale(t *testing.T) {
	brokerageCash := 1000.0
	session := connect(t, func(ctx context.Context) (ReadOnlyAccount, error) {
		return &fakeAccount{totalCash: brokerageCash}, nil
	})

	var summary AccountSummary
	callTool(t, session, "get_account_summary", map[string]any{}, &summary)
	assert.Equal(t, 1000.0, summary.TotalCash)

	brokerageCash = 250
	callTool(t, session, "get_account_summary", map[string]any{}, &summary)
	assert.Equal(t, 250.0, summary.TotalCash)
}
