package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestCalcUnrealizedPnL(t *testing.T) {
	long := &Position{Side: SideLong, Size: 2, EntryPrice: 100}
	if got := long.CalcUnrealizedPnL(110); !approx(got, 20) {
		t.Fatalf("long pnl = %v, want 20", got)
	}
	short := &Position{Side: SideShort, Size: 2, EntryPrice: 100}
	if got := short.CalcUnrealizedPnL(110); !approx(got, -20) {
		t.Fatalf("short pnl = %v, want -20", got)
	}
}

func TestCalcLiquidationPrice(t *testing.T) {
	if got := CalcLiquidationPrice(SideLong, 100, 10); !approx(got, 90.5) {
		t.Fatalf("long liq = %v, want 90.5", got)
	}
	if got := CalcLiquidationPrice(SideShort, 100, 10); !approx(got, 109.5) {
		t.Fatalf("short liq = %v, want 109.5", got)
	}
}

func TestShouldLiquidateAndTPSL(t *testing.T) {
	long := &Position{Side: SideLong, LiquidationPrice: 90, TakeProfit: 120, StopLoss: 95}
	if !long.ShouldLiquidate(89) || long.ShouldLiquidate(91) {
		t.Fatal("long liquidation boundary wrong")
	}
	if !long.ShouldTriggerTPSL(120) || !long.ShouldTriggerTPSL(95) || long.ShouldTriggerTPSL(110) {
		t.Fatal("long TP/SL trigger wrong")
	}
}

func TestOpenPosition_Validation(t *testing.T) {
	if _, err := OpenPosition(1, "BTC_USDT", "UP", 10, 1, 100, 0, 0); !errors.Is(err, ErrInvalidSide) {
		t.Fatalf("bad side want ErrInvalidSide, got %v", err)
	}
	if _, err := OpenPosition(1, "BTC_USDT", SideLong, 200, 1, 100, 0, 0); !errors.Is(err, ErrInvalidLeverage) {
		t.Fatalf("bad leverage want ErrInvalidLeverage, got %v", err)
	}
	if _, err := OpenPosition(1, "BTC_USDT", SideLong, 10, 0, 100, 0, 0); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("bad size want ErrInvalidSize, got %v", err)
	}
	p, err := OpenPosition(1, "BTC_USDT", SideLong, 10, 2, 100, 0, 0)
	if err != nil {
		t.Fatalf("valid open: %v", err)
	}
	if !approx(p.Margin, 20) { // notional 200 / 10
		t.Fatalf("margin = %v, want 20", p.Margin)
	}
	if !approx(p.LiquidationPrice, 90.5) {
		t.Fatalf("liq price = %v, want 90.5", p.LiquidationPrice)
	}
}

func TestClose_RealizedNetOfBothFees(t *testing.T) {
	p := &Position{Side: SideLong, Size: 1, EntryPrice: 100, Margin: 10, Status: StatusOpen}
	st := p.Close(110, time.Now())
	// pnl=10, closeFee=0.055, openFee=0.05
	if !approx(st.NetDelta, 9.945) {
		t.Fatalf("net delta = %v, want 9.945", st.NetDelta)
	}
	if !approx(st.UnlockMargin, 10) {
		t.Fatalf("unlock margin = %v, want 10", st.UnlockMargin)
	}
	if !approx(p.UnrealizedPnL, 9.895) || p.Status != StatusClosed {
		t.Fatalf("realized=%v status=%s, want 9.895/CLOSED", p.UnrealizedPnL, p.Status)
	}
}

func TestClose_LossCappedAtMargin(t *testing.T) {
	// Huge adverse move: net loss must not exceed margin.
	p := &Position{Side: SideLong, Size: 1, EntryPrice: 100, Margin: 10, Status: StatusOpen}
	st := p.Close(50, time.Now()) // pnl=-50
	if !approx(st.NetDelta, -10) {
		t.Fatalf("net delta = %v, want capped -10", st.NetDelta)
	}
}

func TestLiquidate_FullMarginLoss(t *testing.T) {
	p := &Position{Side: SideShort, Size: 1, EntryPrice: 100, Margin: 10, Status: StatusOpen}
	st := p.Liquidate(120, time.Now())
	if !approx(st.NetDelta, -10) || !approx(st.UnlockMargin, 10) {
		t.Fatalf("liquidation settlement wrong: %+v", st)
	}
	if p.Status != StatusLiquidated || !approx(p.UnrealizedPnL, -10) {
		t.Fatalf("status=%s pnl=%v, want LIQUIDATED/-10", p.Status, p.UnrealizedPnL)
	}
}
