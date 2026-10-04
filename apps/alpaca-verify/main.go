package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/alpaca"
)

// This script verifies Alpaca API credentials by fetching account information.
// Unlike Schwab which uses OAuth, Alpaca uses simple API key/secret authentication.
//
// Usage:
//   ALPACA_CLIENT_CONFIG=/path/to/config.json go run main.go
//
// The config file should be a JSON file with the following structure:
//   {
//     "api_key": "your-api-key",
//     "api_secret": "your-api-secret",
//     "base_url": "https://paper-api.alpaca.markets"
//   }
//
// For paper trading (recommended for testing), use: https://paper-api.alpaca.markets
// For live trading, use: https://api.alpaca.markets

func main() {
	configFile := os.Getenv("ALPACA_CLIENT_CONFIG")
	if configFile == "" {
		log.Fatal("ALPACA_CLIENT_CONFIG environment variable not set")
	}

	config, err := alpaca.LoadConfigFromFile(configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	client := alpaca.NewClient(config)

	fmt.Println("Connecting to Alpaca...")
	if client.IsPaperTrading() {
		fmt.Println("Mode: PAPER TRADING")
	} else {
		fmt.Println("Mode: LIVE TRADING")
	}

	ctx := context.Background()

	account, err := alpaca.NewTradingAccount(ctx, client)
	if err != nil {
		log.Fatalf("Failed to connect to Alpaca: %v", err)
	}

	fmt.Println("\nAccount verified successfully!")
	fmt.Println("----------------------------")

	accountType, _ := account.Type(ctx)
	fmt.Printf("Account Type: %s\n", accountType)

	cash, _ := account.TotalCash(ctx)
	fmt.Printf("Total Cash: $%.2f\n", cash)

	buyingPower, _ := account.CashAvailableForTrading(ctx)
	fmt.Printf("Buying Power: $%.2f\n", buyingPower)

	longValue, _ := account.LongMarketValue(ctx)
	fmt.Printf("Long Market Value: $%.2f\n", longValue)

	shortValue, _ := account.ShortMarketValue(ctx)
	fmt.Printf("Short Market Value: $%.2f\n", shortValue)

	positions, err := account.Positions(ctx)
	if err != nil {
		log.Printf("Warning: Could not fetch positions: %v", err)
	} else {
		fmt.Printf("\nPositions: %d\n", len(positions))
		for _, p := range positions {
			fmt.Printf("  %s: %.2f shares @ $%.2f (P/L: $%.2f / %.2f%%)\n",
				p.Symbol, p.Quantity, p.AveragePrice, p.UnrealizedPL, p.UnrealizedPLPct)
		}
	}

	// Test market data
	fmt.Println("\nTesting market data...")
	testSymbols := []string{"AAPL", "MSFT", "GOOGL"}
	prices, err := account.LatestRegularMarketPrices(ctx, testSymbols)
	if err != nil {
		log.Printf("Warning: Could not fetch market prices: %v", err)
	} else {
		fmt.Println("Latest prices:")
		for symbol, price := range prices {
			fmt.Printf("  %s: $%.2f\n", symbol, price)
		}
	}

	fmt.Println("\nAlpaca credentials verified successfully!")
}
