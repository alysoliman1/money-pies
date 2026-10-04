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
	brokerageName := os.Getenv("BROKERAGE")
	if brokerageName == "" {
		fmt.Println("brokerage name not specified")
		return
	}

	tradingAccount, err := getTradingAccount(brokerageName)
	if err != nil {
		fmt.Println(err)
		return
	}

	pieLocation := os.Getenv("PIE_LOCATION")
	if pieLocation == "" {
		fmt.Println("PIE_LOCATION not specified")
		return
	}

	pie, err := getPie(pieLocation)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx := context.Background()
	preInvestedAmounts := map[string]float64{
		"FICO": 1500,
		"AZO":  1500,
	}
	investmentAmount := 150000.0

	i := investor.NewInvestor(tradingAccount)
	if err := i.PlacePieOrderWithoutFractionalShares(
		ctx,
		investmentAmount,
		pie,
		preInvestedAmounts,
	); err != nil {
		fmt.Println("failed to place pie order:", err)
		return
	}

	fmt.Println("pie order placed successfully")
}

func getPie(pieLocation string) (investor.Pie, error) {
	file, err := os.Open(pieLocation)
	if err != nil {
		return investor.Pie{}, fmt.Errorf("failed to open pie file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read all records at once
	records, err := reader.ReadAll()
	if err != nil {
		return investor.Pie{}, fmt.Errorf("failed to read pie file: %v", err)
	}

	// Print the records
	slices := []investor.Slice{}
	for _, record := range records[1:] {
		weight, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return investor.Pie{}, fmt.Errorf("failed to parse weight: %v", err)
		}
		slices = append(slices, investor.Slice{
			Weight: weight,
			Symbol: record[0],
		})
	}

	return investor.Pie{
		Slices: slices,
	}, nil
}

func getTradingAccount(brokerageName string) (investor.TradingAccount, error) {
	configDir := os.Getenv("MONEY_PIES_CONFIG")
	if configDir == "" {
		return nil, errors.New("config directory not specified")
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
