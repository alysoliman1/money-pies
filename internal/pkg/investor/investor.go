package investor

import (
	"context"
	"fmt"
	"math"
)

type Pie struct {
	ID          string
	Name        string
	Description string
	Slices      []Slice
}

func (p *Pie) GetSymbols() []string {
	symbols := []string{}
	for _, slice := range p.Slices {
		symbols = append(symbols, slice.Asset.Symbol)
	}
	return symbols
}

type Slice struct {
	Weight float64
	Asset  Asset
}

type Asset struct {
	TypeName string
	ID       string
	IsActive bool
	Name     string
	Symbol   string
	Status   string
}

type Investor struct {
	Account TradingAccount
}

// NewInvestor creates a new investor with a trading account
func NewInvestor(account TradingAccount) *Investor {
	return &Investor{
		Account: account,
	}
}

// GetPieStatus retrieves the status of a pie
func (i *Investor) GetPieStatus(ctx context.Context, pie Pie) {
	if i.Account == nil {
		return
	}
	positions, err := i.Account.GetPositions(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	positionsMap := make(map[string]Position)
	for _, position := range positions {
		positionsMap[position.Symbol] = position
	}
	for _, slice := range pie.Slices {
		position, ok := positionsMap[slice.Asset.Symbol]
		if !ok {
			fmt.Println("Position not found for symbol:", slice.Asset.Symbol)
			continue
		}
		fmt.Println(position.Symbol, position.Quantity, position.AveragePrice, position.CurrentPrice)
	}
}

// PlacePieOrder places an order for a pie
func (i *Investor) PlacePieOrder(
	ctx context.Context,
	amount float64,
	pie Pie,
) {
	if i.Account == nil {
		return
	}
	totalCash, err := i.Account.GetCashAvailableForTrading(ctx)
	if err != nil {
		fmt.Println("Failed to get cash available for trading")
		return
	}
	if amount <= 0 || amount > totalCash {
		fmt.Println("Amount is not valid")
		return
	}
	symbols := pie.GetSymbols()
	prices, err := i.Account.GetRegularMarketLatestPrices(ctx, symbols)
	if err != nil {
		fmt.Println("Failed to get quotes for symbols:", symbols)
		return
	}
	for _, slice := range pie.Slices {
		sliceAmount := amount * slice.Weight
		if sliceAmount <= 0 {
			fmt.Println("Slice amount is not valid")
			return
		}
		quotePrice := prices[slice.Asset.Symbol]
		if quotePrice <= 0 {
			fmt.Println("Quote price is not valid")
			return
		}
		orderQuantity := math.Round(sliceAmount / quotePrice)
		if orderQuantity <= 0 {
			fmt.Println("Order quantity is not valid")
			return
		}
		fmt.Println("Order quantity:", orderQuantity)
		order := OrderRequest{
			Symbol:   slice.Asset.Symbol,
			Action:   OrderActionBuy,
			Type:     OrderTypeMarket,
			Quantity: orderQuantity,
		}
		_, err = i.Account.PlaceOrder(ctx, order)
		if err != nil {
			fmt.Println(err)
			fmt.Println("Failed to place order for symbol:", slice.Asset.Symbol)
			return
		}
		fmt.Println("Order placed for symbol:", slice.Asset.Symbol)
	}

	// i.BrokerageClient.PlaceOrder(ctx)
}
