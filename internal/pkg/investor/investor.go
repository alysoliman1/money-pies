package investor

type Investor struct {
	Account TradingAccount
}

// NewInvestor creates a new investor with a trading account
func NewInvestor(account TradingAccount) *Investor {
	return &Investor{
		Account: account,
	}
}
