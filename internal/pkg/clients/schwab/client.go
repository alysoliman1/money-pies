package schwab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/investor"
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
	config     Config
	httpClient *http.Client
	token      *Token
}

// NewClient creates a new Schwab client
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production
func NewClient(config Config, timeoutInSeconds int) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", c.config.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	credentials := fmt.Sprintf("%s:%s", c.config.ClientID, c.config.ClientSecret)
	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", encodedCredentials))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
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

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.token.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh token request: %w", err)
	}

	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.config.ClientID, c.config.ClientSecret)))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", encodedCredentials))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read refresh token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return fmt.Errorf("failed to parse refresh token response: %w", err)
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
func (c *Client) makeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Check if token needs refresh
	if c.token != nil && time.Now().Add(5*time.Minute).After(c.token.ExpiresAt) {
		if err := c.refreshToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated")
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	//req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

type TradingAccount struct {
	AccountNumber              string
	HashValue                  string
	Client                     *Client
	Type                       string
	CashAvailableForTrading    float64
	CashAvailableForWithdrawal float64
	TotalCash                  float64
	LongMarketValue            float64
	ShortMarketValue           float64
	PendingDeposits            float64
}

// NewTradingAccount creates a new trading account
func NewTradingAccount(ctx context.Context, client *Client, accountNumber string) (*TradingAccount, error) {
	resp, err := client.makeRequest(ctx, "GET", accountsNumbersPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read accounts response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get accounts failed with status %d: %s", resp.StatusCode, string(body))
	}

	var schwabAccountsNumbers []struct {
		AccountNumber string `json:"accountNumber"`
		HashValue     string `json:"hashValue"`
	}

	if err := json.Unmarshal(body, &schwabAccountsNumbers); err != nil {
		return nil, fmt.Errorf("failed to parse accounts response: %w", err)
	}

	for _, account := range schwabAccountsNumbers {
		if account.AccountNumber == accountNumber {
			resp, err := client.makeRequest(ctx, "GET", fmt.Sprintf("%s/%s", accountsPath, account.HashValue), nil)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read accounts response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("get accounts failed with status %d: %s", resp.StatusCode, string(body))
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
			if err := json.Unmarshal(body, &schwabAccount); err != nil {
				return nil, fmt.Errorf("failed to parse account response: %w", err)
			}

			return &TradingAccount{
				AccountNumber:              account.AccountNumber,
				CashAvailableForTrading:    schwabAccount.SecuritiesAccount.CurrentBalances.CashAvailableForTrading,
				CashAvailableForWithdrawal: schwabAccount.SecuritiesAccount.CurrentBalances.CashAvailableForWithdrawal,
				TotalCash:                  schwabAccount.SecuritiesAccount.CurrentBalances.TotalCash,
				LongMarketValue:            schwabAccount.SecuritiesAccount.CurrentBalances.LongMarketValue,
				ShortMarketValue:           schwabAccount.SecuritiesAccount.CurrentBalances.ShortMarketValue,
				PendingDeposits:            schwabAccount.SecuritiesAccount.CurrentBalances.PendingDeposits,
				Type:                       schwabAccount.SecuritiesAccount.Type,
				HashValue:                  account.HashValue,
				Client:                     client,
			}, nil
		}
	}
	return nil, fmt.Errorf("account number not found")
}

func (c *TradingAccount) GetTotalCash(ctx context.Context) (float64, error) {
	return c.TotalCash, nil
}

func (c *TradingAccount) GetCashAvailableForTrading(ctx context.Context) (float64, error) {
	return c.CashAvailableForTrading, nil
}

func (c *TradingAccount) GetCashAvailableForWithdrawal(ctx context.Context) (float64, error) {
	return c.CashAvailableForWithdrawal, nil
}

func (c *TradingAccount) GetLongMarketValue(ctx context.Context) (float64, error) {
	return c.LongMarketValue, nil
}

func (c *TradingAccount) GetShortMarketValue(ctx context.Context) (float64, error) {
	return c.ShortMarketValue, nil
}

func (c *TradingAccount) GetPendingDeposits(ctx context.Context) (float64, error) {
	return c.PendingDeposits, nil
}

func (c *TradingAccount) GetType(ctx context.Context) (string, error) {
	return c.Type, nil
}

// GetPositions retrieves positions for a specific account
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}
func (c *TradingAccount) GetPositions(ctx context.Context) ([]investor.Position, error) {
	path := fmt.Sprintf("%s/%s?fields=positions", accountsPath, c.HashValue)
	resp, err := c.Client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read positions response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get positions failed with status %d: %s", resp.StatusCode, string(body))
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

	if err := json.Unmarshal(body, &accountData); err != nil {
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
func (c *TradingAccount) PlaceOrder(ctx context.Context, order investor.OrderRequest) (*investor.Order, error) {
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

	orderJSON, err := json.Marshal(schwabOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order: %w", err)
	}

	path := fmt.Sprintf(ordersPath, c.HashValue)
	resp, err := c.Client.makeRequest(ctx, "POST", path, strings.NewReader(string(orderJSON)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read order response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("place order failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Extract order ID from Location header
	orderID := ""
	if location := resp.Header.Get("Location"); location != "" {
		parts := strings.Split(location, "/")
		if len(parts) > 0 {
			orderID = parts[len(parts)-1]
		}
	}

	return &investor.Order{
		ID:          orderID,
		Symbol:      order.Symbol,
		Action:      order.Action,
		Type:        order.Type,
		Quantity:    order.Quantity,
		LimitPrice:  order.LimitPrice,
		Status:      investor.OrderStatusPending,
		SubmittedAt: time.Now(),
		RawResponse: string(body),
	}, nil
}

// GetOrder retrieves a specific order
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}/orders/{orderId}
func (c *TradingAccount) GetOrderStatus(ctx context.Context, orderID string) (*investor.Order, error) {
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, c.HashValue, orderID)
	resp, err := c.Client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read order response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get order failed with status %d: %s", resp.StatusCode, string(body))
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

	if err := json.Unmarshal(body, &schwabOrder); err != nil {
		return nil, fmt.Errorf("failed to parse order response: %w", err)
	}

	order := &investor.Order{
		ID:          fmt.Sprintf("%d", schwabOrder.OrderID),
		Status:      convertOrderStatus(schwabOrder.Status),
		Quantity:    schwabOrder.Quantity,
		FilledQty:   schwabOrder.FilledQuantity,
		FilledPrice: schwabOrder.Price,
		Type:        investor.OrderType(schwabOrder.OrderType),
		RawResponse: string(body),
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
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, c.HashValue, orderID)
	resp, err := c.Client.makeRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel order failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetOrders retrieves recent orders
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}/orders
func (c *TradingAccount) GetRecentOrders(ctx context.Context, limit int) ([]investor.Order, error) {
	path := fmt.Sprintf("%s/%s/orders?maxResults=%d", accountsPath, c.HashValue, limit)
	resp, err := c.Client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read orders response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get orders failed with status %d: %s", resp.StatusCode, string(body))
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

	if err := json.Unmarshal(body, &schwabOrders); err != nil {
		return nil, fmt.Errorf("failed to parse orders response: %w", err)
	}

	orders := make([]investor.Order, 0, len(schwabOrders))
	for _, so := range schwabOrders {
		order := investor.Order{
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
func (c *TradingAccount) GetRegularMarketLatestPrices(ctx context.Context, symbols []string) (map[string]float64, error) {
	path := fmt.Sprintf("%s?symbols=%s", quotesPath, url.QueryEscape(strings.Join(symbols, ",")))
	resp, err := c.Client.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read quote response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get quote failed with status %d: %s", resp.StatusCode, string(body))
	}

	var quotes map[string]struct {
		Regular struct {
			RegularMarketLastPrice float64 `json:"regularMarketLastPrice"`
		} `json:"regular"`
	}
	if err := json.Unmarshal(body, &quotes); err != nil {
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
