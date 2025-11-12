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

	brokerage "github.com/asoliman1/money-pies/internal/pkg/pies"
)

// Schwab API Documentation Links:
// Main API Docs: https://developer.schwab.com/
// OAuth Guide: https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production
// Trading API: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Account & Trading Endpoints: https://api.schwabapi.com/trader/v1/docs/

const (
	// Schwab API endpoints
	baseURL      = "https://api.schwabapi.com"
	authURL      = "https://api.schwabapi.com/v1/oauth/authorize"
	tokenURL     = "https://api.schwabapi.com/v1/oauth/token"
	accountsPath = "/trader/v1/accounts"
	ordersPath   = "/trader/v1/accounts/%s/orders"
	quotesPath   = "/marketdata/v1/quotes"
)

// Config holds Schwab API configuration
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

func (c *Client) SetAccessTokenFromFile() *Client {
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

	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// GetAccounts retrieves all accounts
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts
func (c *Client) GetAccounts(ctx context.Context) ([]brokerage.Account, error) {
	resp, err := c.makeRequest(ctx, "GET", accountsPath, nil)
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

	var schwabAccounts []struct {
		SecuritiesAccount struct {
			AccountNumber   string `json:"accountNumber"`
			Type            string `json:"type"`
			AccountID       string `json:"accountId"`
			CurrentBalances struct {
				CashBalance float64 `json:"cashBalance"`
				BuyingPower float64 `json:"buyingPower"`
				MarketValue float64 `json:"longMarketValue"`
			} `json:"currentBalances"`
		} `json:"securitiesAccount"`
	}

	if err := json.Unmarshal(body, &schwabAccounts); err != nil {
		return nil, fmt.Errorf("failed to parse accounts response: %w", err)
	}

	accounts := make([]brokerage.Account, 0, len(schwabAccounts))
	for _, sa := range schwabAccounts {
		acc := sa.SecuritiesAccount
		accounts = append(accounts, brokerage.Account{
			AccountID:     acc.AccountID,
			AccountNumber: acc.AccountNumber,
			Type:          acc.Type,
			CashBalance:   acc.CurrentBalances.CashBalance,
			BuyingPower:   acc.CurrentBalances.BuyingPower,
			MarketValue:   acc.CurrentBalances.MarketValue,
			TotalValue:    acc.CurrentBalances.CashBalance + acc.CurrentBalances.MarketValue,
		})
	}

	return accounts, nil
}

// GetPositions retrieves positions for a specific account
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}
func (c *Client) GetPositions(ctx context.Context, accountID string) ([]brokerage.Position, error) {
	path := fmt.Sprintf("%s/%s?fields=positions", accountsPath, accountID)
	resp, err := c.makeRequest(ctx, "GET", path, nil)
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

	positions := make([]brokerage.Position, 0, len(accountData.SecuritiesAccount.Positions))
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

		positions = append(positions, brokerage.Position{
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
func (c *Client) PlaceOrder(ctx context.Context, accountID string, order brokerage.OrderRequest) (*brokerage.Order, error) {
	// Build Schwab order structure
	schwabOrder := map[string]interface{}{
		"orderType":         string(order.Type),
		"session":           "NORMAL",
		"duration":          "DAY",
		"orderStrategyType": "SINGLE",
		"orderLegCollection": []map[string]interface{}{
			{
				"instruction": string(order.Action),
				"quantity":    order.Quantity,
				"instrument": map[string]interface{}{
					"symbol":    order.Symbol,
					"assetType": "EQUITY",
				},
			},
		},
	}

	// Add price for limit orders
	if order.Type == brokerage.OrderTypeLimit && order.LimitPrice != nil {
		schwabOrder["price"] = *order.LimitPrice
	}

	orderJSON, err := json.Marshal(schwabOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order: %w", err)
	}

	path := fmt.Sprintf(ordersPath, accountID)
	resp, err := c.makeRequest(ctx, "POST", path, strings.NewReader(string(orderJSON)))
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

	return &brokerage.Order{
		ID:          orderID,
		Symbol:      order.Symbol,
		Action:      order.Action,
		Type:        order.Type,
		Quantity:    order.Quantity,
		LimitPrice:  order.LimitPrice,
		Status:      brokerage.OrderStatusPending,
		SubmittedAt: time.Now(),
		RawResponse: string(body),
	}, nil
}

// GetOrder retrieves a specific order
// Documentation: https://developer.schwab.com/products/trader-api--individual/details/specifications/Retail%20Trader%20API%20Production
// Endpoint: GET /trader/v1/accounts/{accountId}/orders/{orderId}
func (c *Client) GetOrderStatus(ctx context.Context, accountID string, orderID string) (*brokerage.Order, error) {
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, accountID, orderID)
	resp, err := c.makeRequest(ctx, "GET", path, nil)
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

	order := &brokerage.Order{
		ID:          fmt.Sprintf("%d", schwabOrder.OrderID),
		Status:      c.convertOrderStatus(schwabOrder.Status),
		Quantity:    schwabOrder.Quantity,
		FilledQty:   schwabOrder.FilledQuantity,
		FilledPrice: schwabOrder.Price,
		Type:        brokerage.OrderType(schwabOrder.OrderType),
		RawResponse: string(body),
	}

	if len(schwabOrder.OrderLegCollection) > 0 {
		order.Symbol = schwabOrder.OrderLegCollection[0].Instrument.Symbol
		order.Action = brokerage.OrderAction(schwabOrder.OrderLegCollection[0].Instruction)
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
func (c *Client) CancelPendingOrder(ctx context.Context, accountID string, orderID string) error {
	path := fmt.Sprintf("%s/%s/orders/%s", accountsPath, accountID, orderID)
	resp, err := c.makeRequest(ctx, "DELETE", path, nil)
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
func (c *Client) GetRecentOrders(ctx context.Context, accountID string, limit int) ([]brokerage.Order, error) {
	path := fmt.Sprintf("%s/%s/orders?maxResults=%d", accountsPath, accountID, limit)
	resp, err := c.makeRequest(ctx, "GET", path, nil)
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

	orders := make([]brokerage.Order, 0, len(schwabOrders))
	for _, so := range schwabOrders {
		order := brokerage.Order{
			ID:          fmt.Sprintf("%d", so.OrderID),
			Status:      c.convertOrderStatus(so.Status),
			Quantity:    so.Quantity,
			FilledQty:   so.FilledQuantity,
			FilledPrice: so.Price,
			Type:        brokerage.OrderType(so.OrderType),
		}

		if len(so.OrderLegCollection) > 0 {
			order.Symbol = so.OrderLegCollection[0].Instrument.Symbol
			order.Action = brokerage.OrderAction(so.OrderLegCollection[0].Instruction)
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
func (c *Client) GetQuote(ctx context.Context, symbol string) (map[string]interface{}, error) {
	path := fmt.Sprintf("%s?symbols=%s", quotesPath, url.QueryEscape(symbol))
	resp, err := c.makeRequest(ctx, "GET", path, nil)
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

	var quotes map[string]interface{}
	if err := json.Unmarshal(body, &quotes); err != nil {
		return nil, fmt.Errorf("failed to parse quote response: %w", err)
	}

	return quotes, nil
}

// convertOrderStatus converts Schwab order status to our standard status
func (c *Client) convertOrderStatus(status string) brokerage.OrderStatus {
	switch strings.ToUpper(status) {
	case "FILLED":
		return brokerage.OrderStatusFilled
	case "CANCELED", "CANCELLED":
		return brokerage.OrderStatusCancelled
	case "REJECTED":
		return brokerage.OrderStatusRejected
	default:
		return brokerage.OrderStatusPending
	}
}
