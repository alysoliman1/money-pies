package investor

import (
	"fmt"
	"math"
)

// Pie represents a distribution of stocks in a portfolio.
type Pie struct {
	ID          string
	Name        string
	Description string
	Slices      []Slice
}

// Slice represents a stock in the pie.
type Slice struct {
	Weight float64
	Symbol string
}

// ValidatePie validates the pie
// The weights must be between 0 and 100 and the sum of the weights must be 100.
// Each slice must have a unique symbol.
func (p *Pie) ValidatePie() (map[string]float64, []string, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("pie is nil")
	}
	if len(p.Slices) == 0 {
		return nil, nil, fmt.Errorf("pie has no slices")
	}
	totalWeight := 0.0
	symbolToWeightMap := make(map[string]float64)
	for _, slice := range p.Slices {
		if slice.Symbol == "" {
			return nil, nil, fmt.Errorf("some slices are missing a symbol")
		}
		if _, duplicateSymbol := symbolToWeightMap[slice.Symbol]; duplicateSymbol {
			return nil, nil, fmt.Errorf("symbol %s is duplicated in the pie", slice.Symbol)
		}
		if slice.Weight <= 0 || slice.Weight >= 100 {
			return nil, nil, fmt.Errorf("weight for slice %s (%f) is not valid (must be between 0 and 100)", slice.Symbol, slice.Weight)
		}
		totalWeight += slice.Weight
		symbolToWeightMap[slice.Symbol] = slice.Weight / 100.0
	}
	if math.Round(totalWeight) != 100 {
		return nil, nil, fmt.Errorf("total weight is not 100")
	}
	return symbolToWeightMap, p.GetSymbols(), nil
}

// GetSymbols returns all the symbols appearing in the pie.
func (p *Pie) GetSymbols() []string {
	symbols := []string{}
	for _, slice := range p.Slices {
		symbols = append(symbols, slice.Symbol)
	}
	return symbols
}

// PieFromPositions builds a pie whose weights are the positions' shares of the market value held.
// Positions without a positive market value, such as short positions, are left out.
func PieFromPositions(positions []Position) (Pie, error) {
	total := 0.0
	for _, position := range positions {
		if position.MarketValue > 0 {
			total += position.MarketValue
		}
	}
	if total <= 0 {
		return Pie{}, fmt.Errorf("there are no positions with a market value")
	}

	pie := Pie{Name: "Current positions"}
	for _, position := range positions {
		if position.MarketValue > 0 {
			pie.Slices = append(pie.Slices, Slice{
				Symbol: position.Symbol,
				Weight: position.MarketValue / total * 100,
			})
		}
	}
	if len(pie.Slices) < 2 {
		return Pie{}, fmt.Errorf("a pie needs at least two positions")
	}
	return pie, nil
}
