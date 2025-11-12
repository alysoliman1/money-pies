package pies

import (
	"context"
	"fmt"
)

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

func (i *Investor) GetPieStatus(ctx context.Context, pie Pie) {
	if i.BrokerageClient != nil {
		fmt.Println(i.BrokerageClient.GetAccounts(ctx))
	}
}
