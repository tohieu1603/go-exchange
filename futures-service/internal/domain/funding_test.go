package domain

import "testing"

func TestCalcFundingRate_PremiumAndCap(t *testing.T) {
	// 1% premium → capped at +0.5%.
	if got := CalcFundingRate(101, 100); !approx(got, MaxFundingRate) {
		t.Fatalf("rate = %v, want cap %v", got, MaxFundingRate)
	}
	// -1% → capped at -0.5%.
	if got := CalcFundingRate(99, 100); !approx(got, -MaxFundingRate) {
		t.Fatalf("rate = %v, want -%v", got, MaxFundingRate)
	}
	// Small premium within cap passes through.
	if got := CalcFundingRate(100.2, 100); !approx(got, 0.002) {
		t.Fatalf("rate = %v, want 0.002", got)
	}
	if got := CalcFundingRate(100, 0); got != 0 {
		t.Fatalf("zero index → rate 0, got %v", got)
	}
}

func TestFundingAmount_Sign(t *testing.T) {
	// Positive rate: LONG pays (negative), SHORT receives (positive).
	if got := FundingAmount(SideLong, 0.001, 1000); !approx(got, -1) {
		t.Fatalf("long amount = %v, want -1", got)
	}
	if got := FundingAmount(SideShort, 0.001, 1000); !approx(got, 1) {
		t.Fatalf("short amount = %v, want 1", got)
	}
}
