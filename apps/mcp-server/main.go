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
	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/asoliman1/money-pies/internal/pkg/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This command serves the trading account of a brokerage over MCP using the stdio transport.
// Besides reading the account, it can place and cancel real orders.
//
// Environment variables:
//   BROKERAGE             schwab or alpaca (required)
//   MONEY_PIES_CONFIG     config directory (defaults to $HOME/.money-pies)
//   SCHWAB_ACCOUNT_NUMBER account number to trade with (required for schwab)
//
// The brokerage's config is read from $MONEY_PIES_CONFIG/$BROKERAGE/client-config.json.
// For schwab, authenticate first with ./apps/schwab-oauth.

const schwabTimeoutInSeconds = 30

func main() {
	// stdout carries the MCP protocol, so all logging must go to stderr.
	log.SetOutput(os.Stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	account, err := getAccount(ctx)
	if err != nil {
		log.Fatal(err)
	}

	server := mcpserver.New(account)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("mcp server stopped: %v", err)
	}
}

func getAccount(ctx context.Context) (investor.TradingAccount, error) {
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
		return schwabAccount(ctx, clientConfigName)
	case "alpaca":
		return alpacaAccount(ctx, clientConfigName)
	case "":
		return nil, errors.New("BROKERAGE not specified")
	default:
		return nil, fmt.Errorf("unsupported brokerage %q", brokerageName)
	}
}

func schwabAccount(ctx context.Context, clientConfigName string) (investor.TradingAccount, error) {
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

	account, err := schwab.NewTradingAccount(ctx, client, accountNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to create trading account: %v", err)
	}
	return account, nil
}

func alpacaAccount(ctx context.Context, clientConfigName string) (investor.TradingAccount, error) {
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

	account, err := alpaca.NewTradingAccount(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trading account: %v", err)
	}
	return account, nil
}
