package investor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateBuyOrdersRequests(t *testing.T) {
	tests := []struct {
		name               string
		expectedBehavior   string
		amountToInvest     float64
		symbolToWeightMap  map[string]float64
		latestPricesMap    map[string]float64
		preInvestedAmounts map[string]float64
		wantOrders         []OrderRequest
		wantLeftOverAmount float64
		wantMessages       []string
		wantErrs           []error
	}{
		{
			name: "basic happy case",
			expectedBehavior: `
			- Amount to invest is $120, and so $60 is allocated to each symbol (each symbol has 50 percent weight).
			- AAPL price is $30 a share, and so we'd buy 2 shares for $60.
			- NVDA price is $40 a share, and so we'd buy 1 share for $40 (left over amount is $20).
			- Therefore, we will buy 2 shares of AAPL for $60, 1 share of NVDA for $40, and the left over amount will be $20.
			`,
			amountToInvest: 120,
			symbolToWeightMap: map[string]float64{
				"AAPL": 50,
				"NVDA": 50,
			},
			latestPricesMap: map[string]float64{
				"AAPL": 30,
				"NVDA": 40,
			},
			wantOrders: []OrderRequest{
				{
					Symbol:   "AAPL",
					Action:   OrderActionBuy,
					Type:     OrderTypeMarket,
					Quantity: 2,
				},
				{
					Symbol:   "NVDA",
					Action:   OrderActionBuy,
					Type:     OrderTypeMarket,
					Quantity: 1,
				},
			},
			wantLeftOverAmount: 20,
			wantMessages:       []string{},
			wantErrs:           []error{},
		},
		{
			name: "not enough money to invest for symbol",
			expectedBehavior: `
			- Amount to invest is $120, and so $60 is allocated to each symbol (each symbol has 50 percent weight).
			- AAPL price is $30 a share, and so we'd buy 2 shares for $60.
			- NVDA price is $600 a share, and so we wouldn't be able to complete the investment (not enough cash).
			`,
			amountToInvest: 120,
			symbolToWeightMap: map[string]float64{
				"AAPL": 50,
				"NVDA": 50,
			},
			latestPricesMap: map[string]float64{
				"AAPL": 30,
				"NVDA": 600,
			},
			wantMessages: []string{},
			wantErrs:     []error{errors.New("not enough money allocated to symbol NVDA ($60.000000) to buy shares at $600.000000 a share")},
		},
		{
			name: "pre invested amount covers a slice of the pie",
			expectedBehavior: `
			- Amount to invest is $120, and so $60 is allocated to each symbol (each symbol has 50 percent weight).
			- AAPL price is $30 a share, and so we'd buy 2 shares for $60.
			- NVDA price is $600 a share, and so we wouldn't be able to complete the investment (not enough cash).
			- However, NVDA pre invested amount is $80, and so we can get away with not buying any shares for NVDA the $80 covers the required $60 to be invested for NVDA.
			- Therefore, we will buy 1 share of AAPL for $60, and the left over amount will be $0.
			`,
			amountToInvest: 120,
			symbolToWeightMap: map[string]float64{
				"AAPL": 50,
				"NVDA": 50,
			},
			latestPricesMap: map[string]float64{
				"AAPL": 30,
				"NVDA": 600,
			},
			preInvestedAmounts: map[string]float64{
				"NVDA": 80,
			},
			wantOrders: []OrderRequest{
				{
					Symbol:   "AAPL",
					Action:   OrderActionBuy,
					Type:     OrderTypeMarket,
					Quantity: 2,
				},
			},
			wantLeftOverAmount: 0,
			wantMessages: []string{
				"not creating a buy order for symbol NVDA because pre invested amount ($80.000000) is sufficient (amount needed is $60.000000)",
			},
			wantErrs: []error{},
		},
		{
			name: "pre invested amount covers a partial slice of the pie",
			expectedBehavior: `
			- Amount to invest is $120, and so $60 is allocated to each symbol (each symbol has 50 percent weight).
			- AAPL price is $60 a share, and so we'd buy 1 share for $60.
			- NVDA price is $30 a share, and so we'd buy 2 shares for $60.
			- However, NVDA pre invested amount is $30, and so we can get away with buying  only 1 share for $30.
			- Therefore, we will buy 1 share of AAPL for $60, 1 share of NVDA for $30, and the left over amount will be $0.
			`,
			amountToInvest: 120,
			symbolToWeightMap: map[string]float64{
				"AAPL": 50,
				"NVDA": 50,
			},
			latestPricesMap: map[string]float64{
				"AAPL": 60,
				"NVDA": 30,
			},
			preInvestedAmounts: map[string]float64{
				"NVDA": 30,
			},
			wantOrders: []OrderRequest{
				{
					Symbol:   "AAPL",
					Action:   OrderActionBuy,
					Type:     OrderTypeMarket,
					Quantity: 1,
				},
				{
					Symbol:   "NVDA",
					Action:   OrderActionBuy,
					Type:     OrderTypeMarket,
					Quantity: 1,
				},
			},
			wantLeftOverAmount: 0,
			wantMessages:       []string{},
			wantErrs:           []error{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orders, leftOverAmount, messages, errs := GenerateBuyOrdersRequests(test.amountToInvest, test.symbolToWeightMap, test.latestPricesMap, test.preInvestedAmounts)
			assert.Equal(t, test.wantErrs, errs)
			assert.Equal(t, test.wantOrders, orders)
			assert.Equal(t, test.wantLeftOverAmount, leftOverAmount)
			assert.Equal(t, test.wantMessages, messages)
		})
	}
}
