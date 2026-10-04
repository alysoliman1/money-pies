package investor

import (
	"context"
	"time"
)

// ReadOnlyTradingAccount is the part of a trading account integration that only reads from the account.
// Code that depends on it instead of TradingAccount has no way of placing or cancelling orders.
type ReadOnlyTradingAccount interface {
	// RefreshAccount reloads the account's type and balances from the brokerage.
	// The balance and type methods below return the values loaded by the last refresh.
	RefreshAccount(ctx context.Context) error

	// LatestRegularMarketPrices retrieves the latest regular market prices for the given symbols.
	LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error)

	// TotalCash retrieves the total cash amount in the trading account.
	TotalCash(ctx context.Context) (float64, error)

	// CashAvailableForTrading retrieves the cash available for trading in the trading account.
	CashAvailableForTrading(ctx context.Context) (float64, error)

	// CashAvailableForWithdrawal retrieves the cash available for withdrawal in the trading account.
	CashAvailableForWithdrawal(ctx context.Context) (float64, error)

	// LongMarketValue retrieves the long market value in the trading account.
	LongMarketValue(ctx context.Context) (float64, error)

	// ShortMarketValue retrieves the short market value in the trading account.
	ShortMarketValue(ctx context.Context) (float64, error)

	// PendingDeposits retrieves the pending deposits in the trading account.
	PendingDeposits(ctx context.Context) (float64, error)

	// Type retrieves the type of the trading account.
	Type(ctx context.Context) (string, error)

	// Positions retrieves the current positions for the account.
	Positions(ctx context.Context) ([]Position, error)

	// GetOrderStatus retrieves the status of a specific order.
	GetOrderStatus(ctx context.Context, orderID string) (*TradeOrder, error)

	// GetRecentOrders retrieves recent orders for the account.
	GetRecentOrders(ctx context.Context, limit int) ([]TradeOrder, error)
}

// TradingAccount is the interface that all trading account integrations must implement.
type TradingAccount interface {
	ReadOnlyTradingAccount

	// PlaceOrder places a new order for the account.
	PlaceOrder(ctx context.Context, order OrderRequest) (*TradeOrder, error)

	// CancelPendingOrder cancels a pending order.
	CancelPendingOrder(ctx context.Context, orderID string) error
}

// TradeOrder represents a trade order
type TradeOrder struct {
	ID          string
	Symbol      string
	Action      OrderAction
	Type        OrderType
	Quantity    float64
	LimitPrice  *float64 // Only for limit orders
	Status      OrderStatus
	FilledQty   float64
	FilledPrice float64
	SubmittedAt time.Time
	FilledAt    *time.Time
	RawResponse any // Original response from brokerage
}

// OrderType represents the type of order (market, limit, etc.)
type OrderType string

const (
	OrderTypeMarket OrderType = "MARKET"
	OrderTypeLimit  OrderType = "LIMIT"
)

// OrderAction represents buy or sell
type OrderAction string

const (
	OrderActionBuy  OrderAction = "BUY"
	OrderActionSell OrderAction = "SELL"
)

// OrderStatus represents the current status of an order
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusFilled    OrderStatus = "FILLED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
	OrderStatusRejected  OrderStatus = "REJECTED"
)

// OrderRequest represents a request to place an order
type OrderRequest struct {
	Symbol     string
	Action     OrderAction
	Type       OrderType
	Quantity   float64
	LimitPrice *float64 // Required for limit orders
}

// Position represents a current position in a security
type Position struct {
	Symbol          string
	Quantity        float64
	AveragePrice    float64
	CurrentPrice    float64
	MarketValue     float64
	UnrealizedPL    float64
	UnrealizedPLPct float64
}
