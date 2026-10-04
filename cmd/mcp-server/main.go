package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/alpaca"
	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
	"github.com/asoliman1/money-pies/internal/pkg/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This command serves read-only access to the trading account of a brokerage over MCP
// using the stdio transport. It cannot place or cancel orders.
//
// Environment variables:
//   BROKERAGE             schwab or alpaca (required)
//   MONEY_PIES_CONFIG     config directory (defaults to $HOME/.money-pies)
//   SCHWAB_ACCOUNT_NUMBER account number to read (required for schwab)
//
// The brokerage's config is read from $MONEY_PIES_CONFIG/$BROKERAGE/client-config.json.
// For schwab, authenticate first with ./cmd/schwab-oauth.

const schwabTimeoutInSeconds = 30

func main() {
	// stdout carries the MCP protocol, so all logging must go to stderr.
	log.SetOutput(os.Stderr)

	provider, err := getAccountProvider()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Fail fast on bad credentials instead of on the first tool call.
	if _, err := provider(ctx); err != nil {
		log.Fatalf("failed to connect to trading account: %v", err)
	}

	server := mcpserver.New(provider)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("mcp server stopped: %v", err)
	}
}

func getAccountProvider() (mcpserver.AccountProvider, error) {
	configDir := os.Getenv("MONEY_PIES_CONFIG")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to locate config directory: %v", err)
		}
		configDir = filepath.Join(home, ".money-pies")
	}

	brokerageName := os.Getenv("BROKERAGE")
	clientConfigName := filepath.Join(configDir, brokerageName, "client-config.json")

	switch brokerageName {
	case "schwab":
		return schwabAccountProvider(clientConfigName)
	case "alpaca":
		return alpacaAccountProvider(clientConfigName)
	case "":
		return nil, errors.New("BROKERAGE not specified")
	default:
		return nil, fmt.Errorf("unsupported brokerage %q", brokerageName)
	}
}

func schwabAccountProvider(clientConfigName string) (mcpserver.AccountProvider, error) {
	accountNumber := os.Getenv("SCHWAB_ACCOUNT_NUMBER")
	if accountNumber == "" {
		return nil, errors.New("SCHWAB_ACCOUNT_NUMBER not specified")
	}

	rawClientConfig, err := os.ReadFile(clientConfigName)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var clientConfig schwab.Config
	if err := json.Unmarshal(rawClientConfig, &clientConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	client := schwab.
		NewClient(clientConfig, schwabTimeoutInSeconds).
		GetAccessTokenFromFile()

	return func(ctx context.Context) (mcpserver.ReadOnlyAccount, error) {
		return schwab.NewTradingAccount(ctx, client, accountNumber)
	}, nil
}

func alpacaAccountProvider(clientConfigName string) (mcpserver.AccountProvider, error) {
	clientConfig, err := alpaca.LoadConfigFromFile(clientConfigName)
	if err != nil {
		return nil, err
	}

	client := alpaca.NewClient(clientConfig)
	if client.IsPaperTrading() {
		log.Println("alpaca mode: PAPER TRADING")
	} else {
		log.Println("alpaca mode: LIVE TRADING")
	}

	return func(ctx context.Context) (mcpserver.ReadOnlyAccount, error) {
		return alpaca.NewTradingAccount(ctx, client)
	}, nil
}
