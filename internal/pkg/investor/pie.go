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
	pieSymbols := []string{}
	for symbol := range symbolToWeightMap {
		pieSymbols = append(pieSymbols, symbol)
	}
	return symbolToWeightMap, pieSymbols, nil
}

// GetSymbols returns all the symbols appearing in the pie.
func (p *Pie) GetSymbols() []string {
	symbols := []string{}
	for _, slice := range p.Slices {
		symbols = append(symbols, slice.Symbol)
	}
	return symbols
}
