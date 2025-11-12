package pies

import (
	"context"
	"time"
)

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

// Account represents account information
type Account struct {
	AccountID     string
	AccountNumber string
	Type          string
	CashBalance   float64
	BuyingPower   float64
	MarketValue   float64
	TotalValue    float64
}

// Brokerage is the main interface that all brokerage implementations must satisfy
type BrokerageClient interface {
	// IsAuthenticated checks if the client has valid authentication
	IsAuthenticated() bool

	// GetAccounts retrieves all accounts for the authenticated user
	GetAccounts(ctx context.Context) ([]Account, error)

	// GetPositions retrieves all positions for a specific account
	GetPositions(ctx context.Context, accountID string) ([]Position, error)

	// PlaceOrder submits a new order
	PlaceOrder(ctx context.Context, accountID string, order OrderRequest) (*Order, error)

	// GetOrderStatus retrieves the status of a specific order
	GetOrderStatus(ctx context.Context, accountID string, orderID string) (*Order, error)

	// CancelPendingOrder cancels a pending order
	CancelPendingOrder(ctx context.Context, accountID string, orderID string) error

	// GetRecentOrders retrieves recent orders for an account
	GetRecentOrders(ctx context.Context, accountID string, limit int) ([]Order, error)

	// GetQuote retrieves the current quote for a symbol
	GetQuote(ctx context.Context, symbol string) (map[string]any, error)
}
