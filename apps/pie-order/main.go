package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tradingAccount, err := brokerages.NewTradingAccountFromEnv(ctx)
	if err != nil {
		fmt.Println("failed to create trading account:", err)
		return
	}

	pie, err := getPieFromEnv()
	if err != nil {
		fmt.Println("failed to get pie:", err)
		return
	}

	investmentAmount := 1.0
	preInvestedAmounts := map[string]float64{
		// "AAA": 1,
	}

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

func getPieFromEnv() (investor.Pie, error) {
	pieLocation := os.Getenv("PIE_LOCATION")
	if pieLocation == "" {
		return investor.Pie{}, errors.New("PIE_LOCATION not specified")
	}

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
