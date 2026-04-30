package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrder_Remaining(t *testing.T) {
	o := &Order{Amount: 10, FilledAmount: 3}
	assert.InDelta(t, 7.0, o.Remaining(), 1e-9)
}

func TestOrder_Remaining_Zero(t *testing.T) {
	o := &Order{Amount: 5, FilledAmount: 5}
	assert.InDelta(t, 0.0, o.Remaining(), 1e-9)
}

func TestOrder_IsFilled(t *testing.T) {
	cases := []struct {
		name     string
		amount   float64
		filled   float64
		expected bool
	}{
		{"fully filled", 10, 10, true},
		{"partially filled", 10, 7, false},
		{"untouched", 10, 0, false},
		// Defensive: rounding could push filled slightly past amount.
		// Filled >= Amount counts as filled to avoid stuck-open orders.
		{"over-filled (rounding)", 10, 10.0000001, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &Order{Amount: c.amount, FilledAmount: c.filled}
			assert.Equal(t, c.expected, o.IsFilled())
		})
	}
}
