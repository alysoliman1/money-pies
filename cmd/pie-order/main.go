package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

func main() {
	tradingAccount, err := getTradingAccount()
	if err != nil {
		fmt.Println(err)
		return
	}

	pieLocation := os.Getenv("PIE_LOCATION")
	if pieLocation == "" {
		fmt.Println("PIE_LOCATION not specified")
		return
	}

	file, err := os.Open(pieLocation)
	if err != nil {
		fmt.Println("failed to open pie file:", err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read all records at once
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("failed to read pie file:", err)
		return
	}

	// Print the records
	slices := []investor.Slice{}
	for _, record := range records[1:] {
		weight, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			fmt.Println("failed to parse weight:", err)
			return
		}
		slices = append(slices, investor.Slice{
			Weight: weight,
			Symbol: record[0],
		})
	}

	ctx := context.Background()
	preInvestedAmounts := map[string]float64{
		"FICO": 1500,
		"AZO":  1500,
	}

	i := investor.NewInvestor(tradingAccount)
	i.PlacePieOrder(ctx, 150000, investor.Pie{
		Slices: slices,
	}, preInvestedAmounts, false)
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
