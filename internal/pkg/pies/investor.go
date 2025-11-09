package pies

type Pie struct {
	ID          string
	Name        string
	Description string
	Slices      []Slice
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
	Account         Account
	BrokerageClient BrokerageClient
}
