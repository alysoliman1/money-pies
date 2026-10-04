package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/shopspring/decimal"
)

// Alpaca API Documentation Links:
// Main API Docs: https://docs.alpaca.markets/
// Trading API: https://docs.alpaca.markets/docs/trading-api
// Market Data API: https://docs.alpaca.markets/docs/market-data-api
// Go SDK: https://github.com/alpacahq/alpaca-trade-api-go

const (
	// Alpaca API endpoints
	liveBaseURL  = "https://api.alpaca.markets"
	paperBaseURL = "https://paper-api.alpaca.markets"
	dataBaseURL  = "https://data.alpaca.markets"
)

// Config holds Alpaca API configuration
// This is the schema for the alpaca/client-config.json file.
type Config struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	BaseURL   string `json:"base_url"` // Use paper-api.alpaca.markets for paper trading
}

// Client wraps the Alpaca SDK client
type Client struct {
	config           Config
	tradingClient    *alpaca.Client
	marketDataClient *marketdata.Client
}

// NewClient creates a new Alpaca client
func NewClient(config Config) *Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = paperBaseURL // Default to paper trading for safety
	}

	tradingClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    config.APIKey,
		APISecret: config.APISecret,
		BaseURL:   baseURL,
	})

	marketDataClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    config.APIKey,
		APISecret: config.APISecret,
	})

	return &Client{
		config:           config,
		tradingClient:    tradingClient,
		marketDataClient: marketDataClient,
	}
}

// LoadConfigFromFile loads the Alpaca config from a JSON file
func LoadConfigFromFile(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}
	return config, nil
}

// TradingAccount implements the investor.TradingAccount interface for Alpaca
type TradingAccount struct {
	client           *Client
	accountID        string
	status           string
	cash             float64
	longMarketValue  float64
	shortMarketValue float64
	buyingPower      float64
}

// NewTradingAccount creates a new Alpaca trading account
func NewTradingAccount(ctx context.Context, client *Client) (*TradingAccount, error) {
	acct, err := client.tradingClient.GetAccount()
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	cash, _ := acct.Cash.Float64()
	longMarketValue, _ := acct.LongMarketValue.Float64()
	shortMarketValue, _ := acct.ShortMarketValue.Float64()
	buyingPower, _ := acct.BuyingPower.Float64()

	return &TradingAccount{
		client:           client,
		accountID:        acct.ID,
		status:           acct.Status,
		cash:             cash,
		longMarketValue:  longMarketValue,
		shortMarketValue: shortMarketValue,
		buyingPower:      buyingPower,
	}, nil
}

func (t *TradingAccount) TotalCash(ctx context.Context) (float64, error) {
	return t.cash, nil
}

func (t *TradingAccount) CashAvailableForTrading(ctx context.Context) (float64, error) {
	return t.buyingPower, nil
}

func (t *TradingAccount) CashAvailableForWithdrawal(ctx context.Context) (float64, error) {
	// Alpaca doesn't expose a separate cash withdrawable field, using cash as approximation
	return t.cash, nil
}

func (t *TradingAccount) LongMarketValue(ctx context.Context) (float64, error) {
	return t.longMarketValue, nil
}

func (t *TradingAccount) ShortMarketValue(ctx context.Context) (float64, error) {
	return t.shortMarketValue, nil
}

func (t *TradingAccount) PendingDeposits(ctx context.Context) (float64, error) {
	// Alpaca doesn't expose pending transfers in the Account API
	return 0, nil
}

func (t *TradingAccount) Type(ctx context.Context) (string, error) {
	return t.status, nil
}

// Positions retrieves the current positions for the account
func (t *TradingAccount) Positions(ctx context.Context) ([]investor.Position, error) {
	alpacaPositions, err := t.client.tradingClient.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	positions := make([]investor.Position, 0, len(alpacaPositions))
	for _, p := range alpacaPositions {
		qty, _ := p.Qty.Float64()
		avgEntryPrice, _ := p.AvgEntryPrice.Float64()
		currentPrice, _ := p.CurrentPrice.Float64()
		marketValue, _ := p.MarketValue.Float64()
		unrealizedPL, _ := p.UnrealizedPL.Float64()
		unrealizedPLPC, _ := p.UnrealizedPLPC.Float64()

		positions = append(positions, investor.Position{
			Symbol:          p.Symbol,
			Quantity:        qty,
			AveragePrice:    avgEntryPrice,
			CurrentPrice:    currentPrice,
			MarketValue:     marketValue,
			UnrealizedPL:    unrealizedPL,
			UnrealizedPLPct: unrealizedPLPC * 100, // Convert to percentage
		})
	}

	return positions, nil
}

// PlaceOrder submits a new order
func (t *TradingAccount) PlaceOrder(ctx context.Context, order investor.OrderRequest) (*investor.TradeOrder, error) {
	qty := decimal.NewFromFloat(order.Quantity)

	side := alpaca.Buy
	if order.Action == investor.OrderActionSell {
		side = alpaca.Sell
	}

	orderType := alpaca.Market
	if order.Type == investor.OrderTypeLimit {
		orderType = alpaca.Limit
	}

	req := alpaca.PlaceOrderRequest{
		Symbol:      order.Symbol,
		Qty:         &qty,
		Side:        side,
		Type:        orderType,
		TimeInForce: alpaca.Day,
	}

	if order.Type == investor.OrderTypeLimit && order.LimitPrice != nil {
		limitPrice := decimal.NewFromFloat(*order.LimitPrice)
		req.LimitPrice = &limitPrice
	}

	alpacaOrder, err := t.client.tradingClient.PlaceOrder(req)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	filledQty, _ := alpacaOrder.FilledQty.Float64()
	filledAvgPrice, _ := alpacaOrder.FilledAvgPrice.Float64()
	submittedQty, _ := alpacaOrder.Qty.Float64()

	tradeOrder := &investor.TradeOrder{
		ID:          alpacaOrder.ID,
		Symbol:      alpacaOrder.Symbol,
		Action:      order.Action,
		Type:        order.Type,
		Quantity:    submittedQty,
		LimitPrice:  order.LimitPrice,
		Status:      convertOrderStatus(alpacaOrder.Status),
		FilledQty:   filledQty,
		FilledPrice: filledAvgPrice,
		SubmittedAt: alpacaOrder.SubmittedAt,
		RawResponse: alpacaOrder,
	}

	if alpacaOrder.FilledAt != nil {
		tradeOrder.FilledAt = alpacaOrder.FilledAt
	}

	return tradeOrder, nil
}

// GetOrderStatus retrieves the status of a specific order
func (t *TradingAccount) GetOrderStatus(ctx context.Context, orderID string) (*investor.TradeOrder, error) {
	alpacaOrder, err := t.client.tradingClient.GetOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	filledQty, _ := alpacaOrder.FilledQty.Float64()
	filledAvgPrice, _ := alpacaOrder.FilledAvgPrice.Float64()
	submittedQty, _ := alpacaOrder.Qty.Float64()

	var limitPrice *float64
	if alpacaOrder.LimitPrice != nil {
		lp, _ := alpacaOrder.LimitPrice.Float64()
		limitPrice = &lp
	}

	action := investor.OrderActionBuy
	if alpacaOrder.Side == alpaca.Sell {
		action = investor.OrderActionSell
	}

	orderType := investor.OrderTypeMarket
	if alpacaOrder.Type == alpaca.Limit {
		orderType = investor.OrderTypeLimit
	}

	tradeOrder := &investor.TradeOrder{
		ID:          alpacaOrder.ID,
		Symbol:      alpacaOrder.Symbol,
		Action:      action,
		Type:        orderType,
		Quantity:    submittedQty,
		LimitPrice:  limitPrice,
		Status:      convertOrderStatus(alpacaOrder.Status),
		FilledQty:   filledQty,
		FilledPrice: filledAvgPrice,
		SubmittedAt: alpacaOrder.SubmittedAt,
		RawResponse: alpacaOrder,
	}

	if alpacaOrder.FilledAt != nil {
		tradeOrder.FilledAt = alpacaOrder.FilledAt
	}

	return tradeOrder, nil
}

// CancelPendingOrder cancels a pending order
func (t *TradingAccount) CancelPendingOrder(ctx context.Context, orderID string) error {
	if err := t.client.tradingClient.CancelOrder(orderID); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	return nil
}

// GetRecentOrders retrieves recent orders for the account
func (t *TradingAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.TradeOrder, error) {
	status := "all"
	alpacaOrders, err := t.client.tradingClient.GetOrders(alpaca.GetOrdersRequest{
		Status: status,
		Limit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	orders := make([]investor.TradeOrder, 0, len(alpacaOrders))
	for _, o := range alpacaOrders {
		filledQty, _ := o.FilledQty.Float64()
		filledAvgPrice, _ := o.FilledAvgPrice.Float64()
		submittedQty, _ := o.Qty.Float64()

		var limitPrice *float64
		if o.LimitPrice != nil {
			lp, _ := o.LimitPrice.Float64()
			limitPrice = &lp
		}

		action := investor.OrderActionBuy
		if o.Side == alpaca.Sell {
			action = investor.OrderActionSell
		}

		orderType := investor.OrderTypeMarket
		if o.Type == alpaca.Limit {
			orderType = investor.OrderTypeLimit
		}

		tradeOrder := investor.TradeOrder{
			ID:          o.ID,
			Symbol:      o.Symbol,
			Action:      action,
			Type:        orderType,
			Quantity:    submittedQty,
			LimitPrice:  limitPrice,
			Status:      convertOrderStatus(o.Status),
			FilledQty:   filledQty,
			FilledPrice: filledAvgPrice,
			SubmittedAt: o.SubmittedAt,
		}

		if o.FilledAt != nil {
			tradeOrder.FilledAt = o.FilledAt
		}

		orders = append(orders, tradeOrder)
	}

	return orders, nil
}

// LatestRegularMarketPrices retrieves the latest regular market prices for the given symbols
func (t *TradingAccount) LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	trades, err := t.client.marketDataClient.GetLatestTrades(symbols, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get latest trades: %w", err)
	}

	prices := make(map[string]float64)
	for symbol, trade := range trades {
		prices[symbol] = trade.Price
	}

	return prices, nil
}

// DailyClosingPrices retrieves the split-adjusted daily closing prices of a symbol.
func (t *TradingAccount) DailyClosingPrices(ctx context.Context, symbol string, from, to time.Time) ([]investor.DailyPrice, error) {
	bars, err := t.client.marketDataClient.GetBars(symbol, marketdata.GetBarsRequest{
		TimeFrame:  marketdata.OneDay,
		Adjustment: marketdata.Split,
		Start:      from,
		End:        to,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get daily bars for %s: %w", symbol, err)
	}

	prices := make([]investor.DailyPrice, 0, len(bars))
	for _, bar := range bars {
		// Alpaca stamps a daily bar with midnight US Eastern time, which falls on the trading day in UTC.
		day := bar.Timestamp.UTC()
		prices = append(prices, investor.DailyPrice{
			Date:  time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
			Close: bar.Close,
		})
	}
	return prices, nil
}

// convertOrderStatus converts Alpaca order status to our standard status
func convertOrderStatus(status string) investor.OrderStatus {
	switch status {
	case "filled":
		return investor.OrderStatusFilled
	case "canceled", "cancelled", "expired":
		return investor.OrderStatusCancelled
	case "rejected":
		return investor.OrderStatusRejected
	default:
		return investor.OrderStatusPending
	}
}

// RefreshAccount refreshes the account data from Alpaca
func (t *TradingAccount) RefreshAccount(ctx context.Context) error {
	acct, err := t.client.tradingClient.GetAccount()
	if err != nil {
		return fmt.Errorf("failed to refresh account: %w", err)
	}

	t.cash, _ = acct.Cash.Float64()
	t.longMarketValue, _ = acct.LongMarketValue.Float64()
	t.shortMarketValue, _ = acct.ShortMarketValue.Float64()
	t.buyingPower, _ = acct.BuyingPower.Float64()
	t.status = acct.Status

	return nil
}

// IsPaperTrading returns true if the client is configured for paper trading
func (c *Client) IsPaperTrading() bool {
	return c.config.BaseURL == "" || c.config.BaseURL == paperBaseURL
}

// Ensure TradingAccount implements the investor.TradingAccount interface
var _ investor.TradingAccount = (*TradingAccount)(nil)
