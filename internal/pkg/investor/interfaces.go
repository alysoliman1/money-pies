package investor

import (
	"context"
	"time"
)

// TradingAccount is the interface that all trading accounts must satisfy
// It is used to interact with the trading account and place orders
type TradingAccount interface {
	GetRegularMarketLatestPrices(ctx context.Context, symbols []string) (map[string]float64, error)

	// GetTotalCash retrieves the total cash of the account
	GetTotalCash(ctx context.Context) (float64, error)

	// GetCashAvailableForTrading retrieves the cash available for trading of the account
	GetCashAvailableForTrading(ctx context.Context) (float64, error)

	// GetCashAvailableForWithdrawal retrieves the cash available for withdrawal of the account
	GetCashAvailableForWithdrawal(ctx context.Context) (float64, error)

	// GetLongMarketValue retrieves the long market value of the account
	GetLongMarketValue(ctx context.Context) (float64, error)

	// GetShortMarketValue retrieves the short market value of the account
	GetShortMarketValue(ctx context.Context) (float64, error)

	// GetPendingDeposits retrieves the pending deposits of the account
	GetPendingDeposits(ctx context.Context) (float64, error)

	// GetType retrieves the type of the account
	GetType(ctx context.Context) (string, error)

	// GetPositions retrieves the current positions for the account
	GetPositions(ctx context.Context) ([]Position, error)

	// PlaceOrder places a new order for the account
	PlaceOrder(ctx context.Context, order OrderRequest) (*Order, error)

	// GetOrderStatus retrieves the status of a specific order
	GetOrderStatus(ctx context.Context, orderID string) (*Order, error)

	// CancelPendingOrder cancels a pending order
	CancelPendingOrder(ctx context.Context, orderID string) error

	// GetRecentOrders retrieves recent orders for the account
	GetRecentOrders(ctx context.Context, limit int) ([]Order, error)
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

// Order represents a trade order
type Order struct {
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
