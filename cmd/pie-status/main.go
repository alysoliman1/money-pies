package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/asoliman1/money-pies/internal/pkg/clients/schwab"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

func main() {
	tradingAccount, err := getTradingAccount()
	if err != nil {
		fmt.Println(err)
		return
	}

	i := investor.NewInvestor(tradingAccount)
	i.GetPieStatus(context.Background(), investor.Pie{
		Slices: []investor.Slice{
			{
				Asset: investor.Asset{
					Symbol: "NVDA",
				},
			},
			{
				Asset: investor.Asset{
					Symbol: "MTLS",
				},
			},
		},
	})

	i.PlacePieOrder(context.Background(), 200, investor.Pie{
		Slices: []investor.Slice{
			{
				Weight: 0.5,
				Asset: investor.Asset{
					Symbol: "NVDA",
				},
			},
		},
	})
}

func getTradingAccount() (investor.TradingAccount, error) {
	configDir := os.Getenv("MONEY_PIES_CONFIG")
	if configDir == "" {
		return nil, errors.New("config directory not specified")
	}

	brokerageName := os.Getenv("BROKERAGE")
	if brokerageName == "" {
		return nil, errors.New("brokerage name not specified")
	}

	clientConfigName := fmt.Sprintf("%s/%s/client-config.json", configDir, brokerageName)
	rawClientConfig, err := os.ReadFile(clientConfigName)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var clientConfig schwab.Config
	if err := json.Unmarshal(rawClientConfig, &clientConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	accountNumber := "30650409"

	timeoutInSeconds := 30
	client := schwab.
		NewClient(clientConfig, timeoutInSeconds).
		GetAccessTokenFromFile()

	tradingAccount, err := schwab.NewTradingAccount(context.Background(), client, accountNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to create trading account: %v", err)
	}

	return tradingAccount, nil
}
