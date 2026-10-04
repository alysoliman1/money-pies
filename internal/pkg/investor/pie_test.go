package investor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePie(t *testing.T) {
	tests := []struct {
		name        string
		pie         *Pie
		want        map[string]float64
		wantSymbols []string
		err         error
	}{
		{
			name:        "pie is nil",
			pie:         nil,
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("pie is nil"),
		},
		{
			name: "pie with no slices",
			pie: &Pie{
				Slices: []Slice{},
			},
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("pie has no slices"),
		},
		{
			name: "pie with one slice of weight < 100",
			pie: &Pie{
				Slices: []Slice{
					{
						Symbol: "AAPL",
						Weight: 50,
					},
				},
			},
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("total weight is not 100"),
		},
		{
			name: "pie with one slice of weight > 100",
			pie: &Pie{
				Slices: []Slice{
					{
						Symbol: "AAPL",
						Weight: 150,
					},
				},
			},
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("weight for slice AAPL (150.000000) is not valid (must be between 0 and 100)"),
		},
		{
			name: "pie with one slice with missing symbol",
			pie: &Pie{
				Slices: []Slice{
					{
						Weight: 50,
					},
				},
			},
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("some slices are missing a symbol"),
		},
		{
			name: "pie with one slice with duplicate symbol",
			pie: &Pie{
				Slices: []Slice{
					{
						Symbol: "AAPL",
						Weight: 50,
					},
					{
						Symbol: "AAPL",
						Weight: 50,
					},
				},
			},
			want:        nil,
			wantSymbols: nil,
			err:         fmt.Errorf("symbol AAPL is duplicated in the pie"),
		},
		{
			name: "pie with valid slices",
			pie: &Pie{
				Slices: []Slice{
					{
						Symbol: "AAPL",
						Weight: 50,
					},
					{
						Symbol: "NVDA",
						Weight: 50,
					},
				},
			},
			want: map[string]float64{
				"AAPL": 0.5,
				"NVDA": 0.5,
			},
			wantSymbols: []string{"AAPL", "NVDA"},
			err:         nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotSymbols, err := test.pie.ValidatePie()
			assert.Equal(t, test.err, err)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantSymbols, gotSymbols)
		})
	}
}
