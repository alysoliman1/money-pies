# Alpaca Integration

This document describes how to set up and use the Alpaca brokerage integration with Money Pies.

## Overview

[Alpaca](https://alpaca.markets/) is a commission-free stock trading API that's designed for developers. It provides a RESTful API for trading stocks and accessing market data.

### Features
- Commission-free stock and ETF trading
- Paper trading environment for testing
- Real-time market data
- Simple API key authentication

## Prerequisites

1. Create an Alpaca account at [https://alpaca.markets/](https://alpaca.markets/)
2. Generate API keys from your Alpaca dashboard

## Configuration

### 1. Create Configuration Directory

```bash
mkdir -p ~/.money-pies/alpaca
```

### 2. Create Configuration File

Create a file at `~/.money-pies/alpaca/client-config.json` with the following structure:

```json
{
  "api_key": "YOUR_API_KEY_ID",
  "api_secret": "YOUR_API_SECRET_KEY",
  "base_url": "https://paper-api.alpaca.markets"
}
```

### Configuration Fields

| Field | Description | Required |
|-------|-------------|----------|
| `api_key` | Your Alpaca API Key ID | Yes |
| `api_secret` | Your Alpaca API Secret Key | Yes |
| `base_url` | API endpoint URL | No (defaults to paper trading) |

### Base URLs

| Environment | URL |
|-------------|-----|
| Paper Trading (default) | `https://paper-api.alpaca.markets` |
| Live Trading | `https://api.alpaca.markets` |

> **Warning**: Always test with paper trading first before using live trading credentials.

## Verifying Your Setup

Use the verification script to test your Alpaca credentials:

```bash
ALPACA_CLIENT_CONFIG=~/.money-pies/alpaca/client-config.json go run cmd/alpaca-verify/main.go
```

Expected output:
```
Connecting to Alpaca...
Mode: PAPER TRADING

Account verified successfully!
----------------------------
Account Type: ACTIVE
Total Cash: $100000.00
Buying Power: $200000.00
Long Market Value: $0.00
Short Market Value: $0.00

Positions: 0

Testing market data...
Latest prices:
  AAPL: $150.00
  MSFT: $380.00
  GOOGL: $140.00

Alpaca credentials verified successfully!
```

## Usage in Code

### Creating a Client

```go
import (
    "context"
    "github.com/asoliman1/money-pies/internal/pkg/brokerages/alpaca"
)

// Load config from file
config, err := alpaca.LoadConfigFromFile("~/.money-pies/alpaca/client-config.json")
if err != nil {
    log.Fatal(err)
}

// Create client
client := alpaca.NewClient(config)

// Create trading account
ctx := context.Background()
account, err := alpaca.NewTradingAccount(ctx, client)
if err != nil {
    log.Fatal(err)
}
```

### Checking Account Balance

```go
cash, _ := account.TotalCash(ctx)
buyingPower, _ := account.CashAvailableForTrading(ctx)
fmt.Printf("Cash: $%.2f, Buying Power: $%.2f\n", cash, buyingPower)
```

### Getting Positions

```go
positions, err := account.Positions(ctx)
if err != nil {
    log.Fatal(err)
}

for _, p := range positions {
    fmt.Printf("%s: %.2f shares @ $%.2f\n", p.Symbol, p.Quantity, p.AveragePrice)
}
```

### Placing an Order

```go
import "github.com/asoliman1/money-pies/internal/pkg/investor"

order, err := account.PlaceOrder(ctx, investor.OrderRequest{
    Symbol:   "AAPL",
    Action:   investor.OrderActionBuy,
    Type:     investor.OrderTypeMarket,
    Quantity: 1,
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Order placed: %s\n", order.ID)
```

### Getting Market Prices

```go
prices, err := account.LatestRegularMarketPrices(ctx, []string{"AAPL", "MSFT", "GOOGL"})
if err != nil {
    log.Fatal(err)
}

for symbol, price := range prices {
    fmt.Printf("%s: $%.2f\n", symbol, price)
}
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ALPACA_CLIENT_CONFIG` | Path to the Alpaca configuration JSON file |

## API Limitations

- Alpaca does not expose pending deposit information through their API
- Cash available for withdrawal is approximated using the total cash value

## Troubleshooting

### "failed to get account" Error

1. Verify your API keys are correct
2. Ensure the base URL matches your key type (paper vs live)
3. Check that your Alpaca account is active and not restricted

### Market Data Not Available

Market data is only available during market hours or for recently traded securities. For real-time data outside market hours, consider using Alpaca's data subscription.

### Rate Limiting

Alpaca has rate limits on API requests. If you encounter rate limiting errors, add delays between requests or reduce request frequency.

## Resources

- [Alpaca Documentation](https://docs.alpaca.markets/)
- [Alpaca Go SDK](https://github.com/alpacahq/alpaca-trade-api-go)
- [API Reference](https://docs.alpaca.markets/docs/trading-api)
- [Paper Trading Guide](https://docs.alpaca.markets/docs/paper-trading)
