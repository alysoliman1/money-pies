package investor

import (
	"context"
	"fmt"
	"math"
)

type Investor struct {
	Account TradingAccount
}

// NewInvestor creates a new investor with a trading account
func NewInvestor(account TradingAccount) *Investor {
	return &Investor{
		Account: account,
	}
}

// PlacePieOrder places an order for a pie
func (i *Investor) PlacePieOrder(
	ctx context.Context,
	amountToInvest float64,
	pie Pie,
	preInvestedAmounts map[string]float64,
) {
	if i.Account == nil {
		return
	}

	cashAvailableForTrading, err := i.Account.CashAvailableForTrading(ctx)
	if err != nil {
		fmt.Println("failed to get cash available for trading:", err)
		return
	}

	if err := validateAmountToInvest(amountToInvest, cashAvailableForTrading); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("amount to invest: $%f\n", amountToInvest)
	fmt.Printf("cash available for trading: $%f\n", cashAvailableForTrading)

	symbolToWeightMap, err := pie.ValidatePie()
	if err != nil {
		fmt.Println("failed to validate pie:", err)
		return
	}

	pieSymbols := []string{}
	for symbol := range symbolToWeightMap {
		pieSymbols = append(pieSymbols, symbol)
	}

	fmt.Printf("pie validated (%d symbols)\n", len(pieSymbols))

	latestPricesMap, err := i.Account.LatestRegularMarketPrices(ctx, pieSymbols)
	if err != nil {
		fmt.Println("failed to get quotes for symbols:", pieSymbols, ":", err)
		return
	}

	fmt.Printf("quotes retrieved (%d symbols)\n", len(latestPricesMap))

	orders, leftOverAmount, messages, errs := GenerateBuyOrdersRequests(
		amountToInvest,
		symbolToWeightMap,
		latestPricesMap,
		preInvestedAmounts,
	)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Println(err)
		}
		return
	}

	for _, message := range messages {
		fmt.Println(message)
	}

	fmt.Printf("left over amount: $%f\n", leftOverAmount)

	for _, order := range orders {
		fmt.Println("--------------------------------")
		fmt.Println(order.Symbol, order.Quantity, order.Action, order.Type)
		order, err := i.Account.PlaceOrder(ctx, order)
		if err != nil {
			fmt.Println("Failed to place order:", err)
			fmt.Println("--------------------------------")
			continue
		}
		fmt.Println(order.ID, order.Status, order.Quantity, order.Action, order.Type)
		fmt.Println("--------------------------------")
	}
}

func validateAmountToInvest(amountToInvest float64, cashAvailableForTrading float64) error {
	if amountToInvest <= 0 {
		return fmt.Errorf("amount to invest ($%f) is not valid", amountToInvest)
	}
	if amountToInvest > cashAvailableForTrading {
		return fmt.Errorf("amount to invest ($%f) is greater than cash available for trading ($%f)", amountToInvest, cashAvailableForTrading)
	}
	return nil
}

// GenerateOrdersRequests generates the orders requests to invest in the pie.
func GenerateBuyOrdersRequests(
	amountToInvest float64,
	symbolToWeightMap map[string]float64,
	latestPricesMap map[string]float64,
	preInvestedAmounts map[string]float64,
) ([]OrderRequest, float64, []string, []error) {
	leftOverAmount := 0.0
	orders := []OrderRequest{}
	messages := []string{}
	errs := []error{}
	for symbol, weight := range symbolToWeightMap {
		weight /= 100
		if preInvestedAmounts[symbol] >= amountToInvest*weight {
			message := fmt.Sprintf(
				"not creating a buy order for symbol %s because pre invested amount ($%f) is sufficient (amount needed is $%f)",
				symbol,
				preInvestedAmounts[symbol],
				amountToInvest*weight,
			)
			messages = append(messages, message)
			continue
		}

		amountToInvestForSymbol := amountToInvest*weight - preInvestedAmounts[symbol]
		if amountToInvestForSymbol <= 0 {
			err := fmt.Errorf(
				"amount to invest for symbol %s ($%f) is not valid",
				symbol,
				amountToInvestForSymbol,
			)
			errs = append(errs, err)
			continue
		}

		quotePrice := latestPricesMap[symbol]
		if quotePrice <= 0 {
			err := fmt.Errorf(
				"quote price for symbol %s ($%f) is not valid",
				symbol,
				quotePrice,
			)
			errs = append(errs, err)
			continue
		}

		orderQuantity := math.Floor(amountToInvestForSymbol / quotePrice)
		if orderQuantity < 1 {
			err := fmt.Errorf(
				"not enough money allocated to symbol %s ($%f) to buy shares at $%f a share",
				symbol,
				amountToInvestForSymbol,
				quotePrice,
			)
			errs = append(errs, err)
			continue
		}

		leftOverAmount += amountToInvestForSymbol - (orderQuantity * quotePrice)

		orders = append(orders, OrderRequest{
			Symbol:   symbol,
			Action:   OrderActionBuy,
			Type:     OrderTypeMarket,
			Quantity: orderQuantity,
		})
	}
	return orders, leftOverAmount, messages, errs
}
