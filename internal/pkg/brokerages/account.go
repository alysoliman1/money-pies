// Package brokerages connects to the trading account of a supported brokerage.
package brokerages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/alpaca"
	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

const schwabTimeoutInSeconds = 30

// NewTradingAccountFromEnv connects to the trading account described by the environment.
//
// Environment variables:
//
//	BROKERAGE             schwab or alpaca (required)
//	MONEY_PIES_CONFIG     config directory (defaults to $HOME/.money-pies)
//	SCHWAB_ACCOUNT_NUMBER account number to use (required for schwab)
//
// The brokerage's config is read from $MONEY_PIES_CONFIG/$BROKERAGE/client-config.json.
// For schwab, authenticate first with ./apps/schwab-oauth.
func NewTradingAccountFromEnv(ctx context.Context) (investor.TradingAccount, error) {
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
