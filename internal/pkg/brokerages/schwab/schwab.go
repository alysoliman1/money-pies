package schwab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/go-resty/resty/v2"
)

// Schwab API Documentation Links:
// Main API Docs: https://developer.schwab.com/
// OAuth Guide: https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production
// Trading API: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Account & Trading Endpoints: https://api.schwabapi.com/trader/v1/docs/

const (
	// Schwab API endpoints
	baseURL             = "https://api.schwabapi.com"
	authURL             = "https://api.schwabapi.com/v1/oauth/authorize"
	tokenURL            = "https://api.schwabapi.com/v1/oauth/token"
	accountsPath        = "/trader/v1/accounts"
	accountsNumbersPath = "/trader/v1/accounts/accountNumbers"
	ordersPath          = "/trader/v1/accounts/%s/orders"
	quotesPath          = "/marketdata/v1/quotes"
)

// Config holds Schwab API configuration
// This is the schema for the schwab/client-config.json file.
type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	TokenFile    string `json:"token_file"`
}

// Token represents OAuth tokens
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Client implements the brokerage.BrokerageClient interface for Schwab
type Client struct {
	config      Config
	restyClient *resty.Client
	token       *Token
}

// NewClient creates a new Schwab client
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production
func NewClient(config Config, timeoutInSeconds int) *Client {
	client := resty.New().
		SetTimeout(time.Duration(timeoutInSeconds) * time.Second)

	return &Client{
		config:      config,
		restyClient: client,
	}
}

func (c *Client) GetAuthURL() string {
	return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code",
		authURL,
		url.QueryEscape(c.config.ClientID),
		url.QueryEscape(c.config.RedirectURI),
	)
}

func (c *Client) SetAccessToken(token Token) *Client {
	c.token = &token
	rawToken, err := json.Marshal(token)
	if err != nil {
		return c
	}
	os.WriteFile(c.config.TokenFile, rawToken, 0644)
	return c
}

func (c *Client) GetAccessTokenFromFile() *Client {
	rawToken, err := os.ReadFile(c.config.TokenFile)
	if err != nil {
		return c
	}

	var token Token
	if err := json.Unmarshal(rawToken, &token); err != nil {
		fmt.Printf("failed to unmarshal token")
		return c
	}

	c.token = &token
	return c
}

// exchangeCodeForToken exchanges the authorization code for access and refresh tokens
func (c *Client) ExchangeAuthCodeForAccessToken(ctx context.Context, code string) error {
	var token Token
	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetBasicAuth(c.config.ClientID, c.config.ClientSecret).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"grant_type":   "authorization_code",
			"code":         code,
			"redirect_uri": c.config.RedirectURI,
		}).
		SetResult(&token).
		Post(tokenURL)

	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	c.SetAccessToken(token)

	return nil
}

// RefreshToken refreshes the access token using the refresh token
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production
func (c *Client) refreshToken(ctx context.Context) error {
	if c.token == nil || c.token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	var token Token
	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetBasicAuth(c.config.ClientID, c.config.ClientSecret).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": c.token.RefreshToken,
		}).
		SetResult(&token).
		Post(tokenURL)

	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("refresh token request failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	c.SetAccessToken(token)

	return nil
}

// IsAuthenticated checks if the client has a valid access token
func (c *Client) IsAuthenticated() bool {
	return c.token != nil && time.Now().Before(c.token.ExpiresAt)
}

// makeRequest is a helper function to make authenticated API requests
func (c *Client) makeRequest(ctx context.Context, method, path string, body any) (*resty.Response, error) {
	// Check if token needs refresh
	if c.token != nil && time.Now().Add(5*time.Minute).After(c.token.ExpiresAt) {
		if err := c.refreshToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated")
	}

	req := c.restyClient.R().
		SetContext(ctx).
		SetAuthToken(c.token.AccessToken)

	if body != nil {
		req.SetHeader("Content-Type", "application/json").
			SetBody(body)
	}

	var resp *resty.Response
	var err error

	switch method {
	case "GET":
		resp, err = req.Get(baseURL + path)
	case "POST":
		resp, err = req.Post(baseURL + path)
	case "DELETE":
		resp, err = req.Delete(baseURL + path)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

type TradingAccount struct {
	accountNumber              string
	hashValue                  string
	client                     *Client
	accountType                string
	cashAvailableForTrading    float64
	cashAvailableForWithdrawal float64
	totalCash                  float64
	longMarketValue            float64
	shortMarketValue           float64
	pendingDeposits            float64
}

// NewTradingAccount creates a new trading account
func NewTradingAccount(ctx context.Context, client *Client, accountNumber string) (*TradingAccount, error) {
	account := &TradingAccount{
		accountNumber: accountNumber,
		client:        client,
	}
	if err := account.RefreshAccount(ctx); err != nil {
		return nil, err
	}
	return account, nil
}

// RefreshAccount reloads the account's type and balances from Schwab.
// The account number's hash value never changes, so it is only looked up on the first call.
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoints: GET /trader/v1/accounts/accountNumbers and GET /trader/v1/accounts/{accountId}
func (c *TradingAccount) RefreshAccount(ctx context.Context) error {
	if c.hashValue == "" {
		hashValue, err := c.lookupHashValue(ctx)
		if err != nil {
			return err
		}
		c.hashValue = hashValue
	}

	resp, err := c.client.makeRequest(ctx, "GET", fmt.Sprintf("%s/%s", accountsPath, c.hashValue), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("get account failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var schwabAccount struct {
		SecuritiesAccount struct {
			Type            string `json:"type"`
			CurrentBalances struct {
				CashAvailableForTrading    float64 `json:"cashAvailableForTrading"`
				CashAvailableForWithdrawal float64 `json:"cashAvailableForWithdrawal"`
				TotalCash                  float64 `json:"totalCash"`
				LongMarketValue            float64 `json:"longMarketValue"`
				ShortMarketValue           float64 `json:"shortMarketValue"`
				PendingDeposits            float64 `json:"pendingDeposits"`
			} `json:"currentBalances"`
		} `json:"securitiesAccount"`
	}
	if err := json.Unmarshal(resp.Body(), &schwabAccount); err != nil {
		return fmt.Errorf("failed to parse account response: %w", err)
	}

	c.accountType = schwabAccount.SecuritiesAccount.Type
	c.cashAvailableForTrading = schwabAccount.SecuritiesAccount.CurrentBalances.CashAvailableForTrading
	c.cashAvailableForWithdrawal = schwabAccount.SecuritiesAccount.CurrentBalances.CashAvailableForWithdrawal
	c.totalCash = schwabAccount.SecuritiesAccount.CurrentBalances.TotalCash
	c.longMarketValue = schwabAccount.SecuritiesAccount.CurrentBalances.LongMarketValue
	c.shortMarketValue = schwabAccount.SecuritiesAccount.CurrentBalances.ShortMarketValue
	c.pendingDeposits = schwabAccount.SecuritiesAccount.CurrentBalances.PendingDeposits

	return nil
}

// lookupHashValue retrieves the hash value that Schwab uses to identify the account number.
func (c *TradingAccount) lookupHashValue(ctx context.Context) (string, error) {
	resp, err := c.client.makeRequest(ctx, "GET", accountsNumbersPath, nil)
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("get accounts failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var schwabAccountsNumbers []struct {
		AccountNumber string `json:"accountNumber"`
		HashValue     string `json:"hashValue"`
	}

	if err := json.Unmarshal(resp.Body(), &schwabAccountsNumbers); err != nil {
		return "", fmt.Errorf("failed to parse accounts response: %w", err)
	}

	for _, account := range schwabAccountsNumbers {
		if account.AccountNumber == c.accountNumber {
			return account.HashValue, nil
		}
	}
	return "", fmt.Errorf("account number not found")
}

func (c *TradingAccount) TotalCash(ctx context.Context) (float64, error) {
	return c.totalCash, nil
}

func (c *TradingAccount) CashAvailableForTrading(ctx context.Context) (float64, error) {
	return c.cashAvailableForTrading, nil
}

func (c *TradingAccount) CashAvailableForWithdrawal(ctx context.Context) (float64, error) {
	return c.cashAvailableForWithdrawal, nil
}

func (c *TradingAccount) LongMarketValue(ctx context.Context) (float64, error) {
	return c.longMarketValue, nil
}

func (c *TradingAccount) ShortMarketValue(ctx context.Context) (float64, error) {
	return c.shortMarketValue, nil
}

func (c *TradingAccount) PendingDeposits(ctx context.Context) (float64, error) {
	return c.pendingDeposits, nil
}

func (c *TradingAccount) Type(ctx context.Context) (string, error) {
	return c.accountType, nil
}

// GetPositions retrieves positions for a specific account
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}
func (c *TradingAccount) Positions(ctx context.Context) ([]investor.Position, error) {
	path := fmt.Sprintf("%s/%s?fields=positions", accountsPath, c.hashValue)
	resp, err := c.client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get positions failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var accountData struct {
		SecuritiesAccount struct {
			Positions []struct {
				ShortQuantity        float64 `json:"shortQuantity"`
				AveragePrice         float64 `json:"averagePrice"`
				CurrentDayProfitLoss float64 `json:"currentDayProfitLoss"`
				LongQuantity         float64 `json:"longQuantity"`
				MarketValue          float64 `json:"marketValue"`
				Instrument           struct {
					Symbol string `json:"symbol"`
				} `json:"instrument"`
			} `json:"positions"`
		} `json:"securitiesAccount"`
	}

	if err := json.Unmarshal(resp.Body(), &accountData); err != nil {
		return nil, fmt.Errorf("failed to parse positions response: %w", err)
	}

	positions := make([]investor.Position, 0, len(accountData.SecuritiesAccount.Positions))
	for _, p := range accountData.SecuritiesAccount.Positions {
		quantity := p.LongQuantity - p.ShortQuantity
		currentPrice := 0.0
		if quantity != 0 {
			currentPrice = p.MarketValue / quantity
		}

		unrealizedPL := p.MarketValue - (p.AveragePrice * quantity)
		unrealizedPLPct := 0.0
		if p.AveragePrice != 0 {
			unrealizedPLPct = (unrealizedPL / (p.AveragePrice * quantity)) * 100
		}

		positions = append(positions, investor.Position{
			Symbol:          p.Instrument.Symbol,
			Quantity:        quantity,
			AveragePrice:    p.AveragePrice,
			CurrentPrice:    currentPrice,
			MarketValue:     p.MarketValue,
			UnrealizedPL:    unrealizedPL,
			UnrealizedPLPct: unrealizedPLPct,
		})
	}

	return positions, nil
}

// PlaceOrder submits a new order
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: POST /trader/v1/accounts/{accountId}/orders
func (c *TradingAccount) PlaceOrder(ctx context.Context, order investor.OrderRequest) (*investor.TradeOrder, error) {
	// Build Schwab order structure
	schwabOrder := map[string]any{
		"orderType":         string(order.Type),
		"session":           "NORMAL",
		"duration":          "DAY",
		"orderStrategyType": "SINGLE",
		"orderLegCollection": []map[string]any{
			{
				"instruction": string(order.Action),
				"quantity":    order.Quantity,
				"instrument": map[string]any{
					"symbol":    order.Symbol,
					"assetType": "EQUITY",
				},
			},
		},
	}

	// Add price for limit orders
	if order.Type == investor.OrderTypeLimit && order.LimitPrice != nil {
		schwabOrder["price"] = *order.LimitPrice
	}

	path := fmt.Sprintf(ordersPath, c.hashValue)
	resp, err := c.client.makeRequest(ctx, "POST", path, schwabOrder)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("place order failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	// Extract order ID from Location header
	orderID := ""
	if location := resp.Header().Get("Location"); location != "" {
		parts := strings.Split(location, "/")
		if len(parts) > 0 {
			orderID = parts[len(parts)-1]
		}
	}

	return &investor.TradeOrder{
		ID:          orderID,
		Symbol:      order.Symbol,
		Action:      order.Action,
		Type:        order.Type,
		Quantity:    order.Quantity,
		LimitPrice:  order.LimitPrice,
		Status:      investor.OrderStatusPending,
		SubmittedAt: time.Now(),
		RawResponse: string(resp.Body()),
	}, nil
}

// GetOrder retrieves a specific order
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}/orders/{orderId}
func (c *TradingAccount) GetOrderStatus(ctx context.Context, orderID string) (*investor.TradeOrder, error) {
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, c.hashValue, orderID)
	resp, err := c.client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get order failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var schwabOrder struct {
		OrderID            int64   `json:"orderId"`
		Status             string  `json:"status"`
		Quantity           float64 `json:"quantity"`
		FilledQuantity     float64 `json:"filledQuantity"`
		Price              float64 `json:"price"`
		OrderType          string  `json:"orderType"`
		EnteredTime        string  `json:"enteredTime"`
		OrderLegCollection []struct {
			Instruction string `json:"instruction"`
			Instrument  struct {
				Symbol string `json:"symbol"`
			} `json:"instrument"`
		} `json:"orderLegCollection"`
	}

	if err := json.Unmarshal(resp.Body(), &schwabOrder); err != nil {
		return nil, fmt.Errorf("failed to parse order response: %w", err)
	}

	order := &investor.TradeOrder{
		ID:          fmt.Sprintf("%d", schwabOrder.OrderID),
		Status:      convertOrderStatus(schwabOrder.Status),
		Quantity:    schwabOrder.Quantity,
		FilledQty:   schwabOrder.FilledQuantity,
		FilledPrice: schwabOrder.Price,
		Type:        investor.OrderType(schwabOrder.OrderType),
		RawResponse: string(resp.Body()),
	}

	if len(schwabOrder.OrderLegCollection) > 0 {
		order.Symbol = schwabOrder.OrderLegCollection[0].Instrument.Symbol
		order.Action = investor.OrderAction(schwabOrder.OrderLegCollection[0].Instruction)
	}

	if schwabOrder.EnteredTime != "" {
		if t, err := time.Parse(time.RFC3339, schwabOrder.EnteredTime); err == nil {
			order.SubmittedAt = t
		}
	}

	return order, nil
}

// CancelOrder cancels a pending order
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: DELETE /trader/v1/accounts/{accountId}/orders/{orderId}
func (c *TradingAccount) CancelPendingOrder(ctx context.Context, orderID string) error {
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, c.hashValue, orderID)
	resp, err := c.client.makeRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return fmt.Errorf("cancel order failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	return nil
}

// GetOrders retrieves recent orders
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}/orders
func (c *TradingAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.TradeOrder, error) {
	path := fmt.Sprintf("%s/%s/orders?maxResults=%d", accountsPath, c.hashValue, limit)
	resp, err := c.client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get orders failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var schwabOrders []struct {
		OrderID            int64   `json:"orderId"`
		Status             string  `json:"status"`
		Quantity           float64 `json:"quantity"`
		FilledQuantity     float64 `json:"filledQuantity"`
		Price              float64 `json:"price"`
		OrderType          string  `json:"orderType"`
		EnteredTime        string  `json:"enteredTime"`
		OrderLegCollection []struct {
			Instruction string `json:"instruction"`
			Instrument  struct {
				Symbol string `json:"symbol"`
			} `json:"instrument"`
		} `json:"orderLegCollection"`
	}

	if err := json.Unmarshal(resp.Body(), &schwabOrders); err != nil {
		return nil, fmt.Errorf("failed to parse orders response: %w", err)
	}

	orders := make([]investor.TradeOrder, 0, len(schwabOrders))
	for _, so := range schwabOrders {
		order := investor.TradeOrder{
			ID:          fmt.Sprintf("%d", so.OrderID),
			Status:      convertOrderStatus(so.Status),
			Quantity:    so.Quantity,
			FilledQty:   so.FilledQuantity,
			FilledPrice: so.Price,
			Type:        investor.OrderType(so.OrderType),
		}

		if len(so.OrderLegCollection) > 0 {
			order.Symbol = so.OrderLegCollection[0].Instrument.Symbol
			order.Action = investor.OrderAction(so.OrderLegCollection[0].Instruction)
		}

		if so.EnteredTime != "" {
			if t, err := time.Parse(time.RFC3339, so.EnteredTime); err == nil {
				order.SubmittedAt = t
			}
		}

		orders = append(orders, order)
	}

	return orders, nil
}

// GetQuote retrieves a quote for a symbol
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /marketdata/v1/quotes
func (c *TradingAccount) LatestRegularMarketPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	path := fmt.Sprintf("%s?symbols=%s", quotesPath, url.QueryEscape(strings.Join(symbols, ",")))
	resp, err := c.client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get quote failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var quotes map[string]struct {
		Regular struct {
			RegularMarketLastPrice float64 `json:"regularMarketLastPrice"`
		} `json:"regular"`
	}
	if err := json.Unmarshal(resp.Body(), &quotes); err != nil {
		return nil, fmt.Errorf("failed to parse quote response: %w", err)
	}
	prices := make(map[string]float64)
	for symbol, quote := range quotes {
		prices[symbol] = quote.Regular.RegularMarketLastPrice
	}

	return prices, nil
}

// convertOrderStatus converts Schwab order status to our standard status
func convertOrderStatus(status string) investor.OrderStatus {
	switch strings.ToUpper(status) {
	case "FILLED":
		return investor.OrderStatusFilled
	case "CANCELED", "CANCELLED":
		return investor.OrderStatusCancelled
	case "REJECTED":
		return investor.OrderStatusRejected
	default:
		return investor.OrderStatusPending
	}
}
