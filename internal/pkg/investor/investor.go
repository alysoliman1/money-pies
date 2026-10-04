package investor

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// floatTolerance absorbs floating point error when comparing dollar amounts.
const floatTolerance = 1e-9

type Investor struct {
	Account TradingAccount
}

// NewInvestor creates a new investor with a trading account
func NewInvestor(account TradingAccount) *Investor {
	return &Investor{
		Account: account,
	}
}

// PlacePieOrderWithoutFractionalShares invests up to amount in the pie using whole shares only.
// preInvestedAmounts holds the dollar amount already invested in each symbol, which counts
// towards the symbol's target of amount * weight.
//
// No order is placed if any of the inputs or prices are invalid. If placing an order fails,
// the remaining orders are still placed and the failures are returned together.
func (i *Investor) PlacePieOrderWithoutFractionalShares(
	ctx context.Context,
	amount float64,
	pie Pie,
	preInvestedAmounts map[string]float64,
) error {
	if i.Account == nil {
		return errors.New("investor has no trading account")
	}

	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("amount to invest ($%f) is not valid", amount)
	}

	symbolToWeightMap, pieSymbols, err := pie.ValidatePie()
	if err != nil {
		return fmt.Errorf("failed to validate pie: %w", err)
	}

	for symbol, preInvestedAmount := range preInvestedAmounts {
		if _, inPie := symbolToWeightMap[symbol]; !inPie {
			return fmt.Errorf("pre invested symbol %s is not in the pie", symbol)
		}
		if preInvestedAmount < 0 || math.IsNaN(preInvestedAmount) || math.IsInf(preInvestedAmount, 0) {
			return fmt.Errorf("pre invested amount for symbol %s ($%f) is not valid", symbol, preInvestedAmount)
		}
	}

	cashAvailableForTrading, err := i.Account.CashAvailableForTrading(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cash available for trading: %w", err)
	}
	if amount > cashAvailableForTrading {
		return fmt.Errorf("amount to invest ($%f) is greater than cash available for trading ($%f)", amount, cashAvailableForTrading)
	}

	latestPricesMap, err := i.Account.LatestRegularMarketPrices(ctx, pieSymbols)
	if err != nil {
		return fmt.Errorf("failed to get prices for symbols %v: %w", pieSymbols, err)
	}
	for _, symbol := range pieSymbols {
		price := latestPricesMap[symbol]
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return fmt.Errorf("price for symbol %s ($%f) is not valid", symbol, price)
		}
	}

	quantities := wholeShareQuantities(amount, pieSymbols, symbolToWeightMap, latestPricesMap, preInvestedAmounts)

	var errs []error
	for _, symbol := range pieSymbols {
		if quantities[symbol] == 0 {
			continue
		}
		_, err := i.Account.PlaceOrder(ctx, OrderRequest{
			Symbol:   symbol,
			Action:   OrderActionBuy,
			Type:     OrderTypeMarket,
			Quantity: quantities[symbol],
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to place order for %v shares of %s: %w", quantities[symbol], symbol, err))
		}
	}
	return errors.Join(errs...)
}

// wholeShareQuantities returns how many whole shares of each symbol to buy with amount.
// weights are fractions that sum to 1 and every symbol must have a positive price.
//
// Each symbol first gets the whole shares that fit in what it is missing from its target
// (amount * weight - pre invested amount). What is left of amount is then spent one share
// at a time on the affordable symbol that ends up the least above its target, with ties
// going to the symbol that comes first in symbols. The amount left over is therefore
// always less than the cheapest share price.
func wholeShareQuantities(
	amount float64,
	symbols []string,
	weights map[string]float64,
	prices map[string]float64,
	preInvestedAmounts map[string]float64,
) map[string]float64 {
	quantities := make(map[string]float64, len(symbols))
	remaining := amount

	// overTarget is how far above its target a symbol is, and is negative while below it.
	overTarget := make(map[string]float64, len(symbols))
	for _, symbol := range symbols {
		needed := math.Max(amount*weights[symbol]-preInvestedAmounts[symbol], 0)
		quantity := math.Floor(needed/prices[symbol] + floatTolerance)
		quantities[symbol] = quantity
		remaining -= quantity * prices[symbol]
		overTarget[symbol] = preInvestedAmounts[symbol] + quantity*prices[symbol] - amount*weights[symbol]
	}

	for {
		best := ""
		for _, symbol := range symbols {
			if prices[symbol] > remaining+floatTolerance {
				continue
			}
			if best == "" || overTarget[symbol]+prices[symbol] < overTarget[best]+prices[best]-floatTolerance {
				best = symbol
			}
		}
		if best == "" {
			return quantities
		}
		quantities[best]++
		remaining -= prices[best]
		overTarget[best] += prices[best]
	}
}
