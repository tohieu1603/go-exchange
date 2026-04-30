package model

import (
	"math"
	"testing"
)

const eps = 1e-6

func approxEq(a, b float64) bool { return math.Abs(a-b) < eps }

func TestCalcUnrealizedPnL_Long(t *testing.T) {
	// LONG profits when mark > entry: pnl = size * (mark - entry).
	p := &FuturesPosition{Side: "LONG", Size: 0.5, EntryPrice: 30000}
	if got := p.CalcUnrealizedPnL(31000); !approxEq(got, 500) {
		t.Errorf("long profit: want 500 got %f", got)
	}
	if got := p.CalcUnrealizedPnL(29000); !approxEq(got, -500) {
		t.Errorf("long loss: want -500 got %f", got)
	}
	if got := p.CalcUnrealizedPnL(30000); !approxEq(got, 0) {
		t.Errorf("long flat: want 0 got %f", got)
	}
}

func TestCalcUnrealizedPnL_Short(t *testing.T) {
	// SHORT profits when mark < entry: pnl = size * (entry - mark).
	p := &FuturesPosition{Side: "SHORT", Size: 0.5, EntryPrice: 30000}
	if got := p.CalcUnrealizedPnL(29000); !approxEq(got, 500) {
		t.Errorf("short profit: want 500 got %f", got)
	}
	if got := p.CalcUnrealizedPnL(31000); !approxEq(got, -500) {
		t.Errorf("short loss: want -500 got %f", got)
	}
}

func TestCalcLiquidationPrice_Long(t *testing.T) {
	// LONG @ 1x leverage: maintenance margin only ⇒ liq ≈ entry * 0.005.
	got := CalcLiquidationPrice("LONG", 30000, 1)
	want := 30000.0 * (1 - 1.0 + 0.005)
	if !approxEq(got, want) {
		t.Errorf("long 1x: want %f got %f", want, got)
	}

	// LONG @ 10x leverage: liq ≈ entry * (1 - 0.1 + 0.005) = entry * 0.905.
	got = CalcLiquidationPrice("LONG", 30000, 10)
	want = 30000.0 * 0.905
	if !approxEq(got, want) {
		t.Errorf("long 10x: want %f got %f", want, got)
	}
}

func TestCalcLiquidationPrice_Short(t *testing.T) {
	// SHORT @ 10x leverage: liq ≈ entry * (1 + 0.1 - 0.005) = entry * 1.095.
	got := CalcLiquidationPrice("SHORT", 30000, 10)
	want := 30000.0 * 1.095
	if !approxEq(got, want) {
		t.Errorf("short 10x: want %f got %f", want, got)
	}
}

func TestCalcLiquidationPrice_HigherLeverageMeansCloserToEntry(t *testing.T) {
	// Sanity: 100x liquidation must be much closer to entry than 2x
	// (less margin to absorb price moves). Distance = entry - liq for LONG.
	lowLevDist := 30000.0 - CalcLiquidationPrice("LONG", 30000, 2)
	highLevDist := 30000.0 - CalcLiquidationPrice("LONG", 30000, 100)
	if highLevDist >= lowLevDist {
		t.Errorf("expected 100x liq closer to entry than 2x: dist 100x=%.2f 2x=%.2f", highLevDist, lowLevDist)
	}
}
