package schwab

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSchwabOrder(t *testing.T, raw string) investor.TradeOrder {
	t.Helper()
	var order schwabOrder
	require.NoError(t, json.Unmarshal([]byte(raw), &order))
	return order.toTradeOrder()
}

func TestToTradeOrderFilledMarketOrder(t *testing.T) {
	order := parseSchwabOrder(t, `{
		"orderType": "MARKET",
		"quantity": 10.0172,
		"filledQuantity": 10.0172,
		"orderLegCollection": [{"instrument": {"symbol": "NVDA"}, "instruction": "SELL"}],
		"orderId": 1007804022432,
		"status": "FILLED",
		"enteredTime": "2026-09-02T17:21:58+0000",
		"closeTime": "2026-09-02T17:22:03+0000",
		"orderActivityCollection": [
			{"activityType": "EXECUTION", "executionLegs": [{"quantity": 10.0, "price": 224.93}]},
			{"activityType": "EXECUTION", "executionLegs": [{"quantity": 0.0172, "price": 224.93}]}
		]
	}`)

	assert.Equal(t, "1007804022432", order.ID)
	assert.Equal(t, "NVDA", order.Symbol)
	assert.Equal(t, investor.OrderActionSell, order.Action)
	assert.Equal(t, investor.OrderTypeMarket, order.Type)
	assert.Equal(t, investor.OrderStatusFilled, order.Status)
	assert.Equal(t, 10.0172, order.FilledQty)
	assert.Equal(t, 224.93, order.FilledPrice)
	assert.Nil(t, order.LimitPrice)
	assert.True(t, order.SubmittedAt.Equal(time.Date(2026, 9, 2, 17, 21, 58, 0, time.UTC)))
	require.NotNil(t, order.FilledAt)
	assert.True(t, order.FilledAt.Equal(time.Date(2026, 9, 2, 17, 22, 3, 0, time.UTC)))
}

func TestToTradeOrderAveragesExecutionsAtDifferentPrices(t *testing.T) {
	order := parseSchwabOrder(t, `{
		"orderType": "MARKET",
		"status": "FILLED",
		"orderActivityCollection": [
			{"activityType": "EXECUTION", "executionLegs": [{"quantity": 30, "price": 100}]},
			{"activityType": "EXECUTION", "executionLegs": [{"quantity": 10, "price": 104}]},
			{"activityType": "ORDER_ACTION", "executionLegs": [{"quantity": 1000, "price": 1}]}
		]
	}`)

	assert.InDelta(t, 101, order.FilledPrice, 1e-9)
}

func TestToTradeOrderPendingOrder(t *testing.T) {
	order := parseSchwabOrder(t, `{
		"orderType": "MARKET",
		"quantity": 17.0,
		"filledQuantity": 0.0,
		"orderLegCollection": [{"instrument": {"symbol": "CTSH"}, "instruction": "SELL"}],
		"orderId": 1008164032040,
		"status": "PENDING_ACTIVATION",
		"enteredTime": "2026-10-04T06:20:14+0000"
	}`)

	assert.Equal(t, investor.OrderStatusPending, order.Status)
	assert.Equal(t, 0.0, order.FilledPrice)
	assert.Nil(t, order.FilledAt)
	assert.True(t, order.SubmittedAt.Equal(time.Date(2026, 10, 4, 6, 20, 14, 0, time.UTC)))
}

func TestToTradeOrderLimitOrder(t *testing.T) {
	order := parseSchwabOrder(t, `{"orderType": "LIMIT", "price": 99.5, "status": "WORKING"}`)

	require.NotNil(t, order.LimitPrice)
	assert.Equal(t, 99.5, *order.LimitPrice)
	// The limit price is not a fill price.
	assert.Equal(t, 0.0, order.FilledPrice)
}

func TestToTradeOrderCancelledOrderHasNoFillTime(t *testing.T) {
	order := parseSchwabOrder(t, `{"orderType": "MARKET", "status": "CANCELED", "closeTime": "2026-09-02T17:22:03+0000"}`)

	assert.Equal(t, investor.OrderStatusCancelled, order.Status)
	assert.Nil(t, order.FilledAt)
}

func TestParseOrderTime(t *testing.T) {
	want := time.Date(2026, 9, 2, 17, 21, 58, 0, time.UTC)

	for _, value := range []string{"2026-09-02T17:21:58+0000", "2026-09-02T17:21:58Z", "2026-09-02T13:21:58-04:00"} {
		got, ok := parseOrderTime(value)
		require.True(t, ok, value)
		assert.True(t, got.Equal(want), value)
	}

	for _, value := range []string{"", "yesterday"} {
		_, ok := parseOrderTime(value)
		assert.False(t, ok, value)
	}
}

func TestParsePriceHistory(t *testing.T) {
	// 1759381200000 is 2025-10-02T05:00:00Z, midnight US Central time.
	prices, err := parsePriceHistory([]byte(`{
		"candles": [
			{"open": 2, "high": 3, "low": 1, "close": 67.26, "volume": 10, "datetime": 1759381200000},
			{"open": 2, "high": 3, "low": 1, "close": 68.26, "volume": 10, "datetime": 1759294800000}
		],
		"symbol": "VYLR",
		"empty": false
	}`))

	require.NoError(t, err)
	assert.Equal(t, []investor.DailyPrice{
		{Date: time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC), Close: 68.26},
		{Date: time.Date(2025, 10, 2, 0, 0, 0, 0, time.UTC), Close: 67.26},
	}, prices)
}

func TestParsePriceHistoryUnknownSymbol(t *testing.T) {
	prices, err := parsePriceHistory([]byte(`{"candles": [], "symbol": "BRK.B", "empty": true}`))

	require.NoError(t, err)
	assert.Empty(t, prices)
}

func TestParsePriceHistoryInvalidBody(t *testing.T) {
	_, err := parsePriceHistory([]byte(`not json`))

	assert.Error(t, err)
}
