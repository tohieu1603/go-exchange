// Package domain holds the perpetual-futures domain: the Position aggregate
// (with all PnL, liquidation and settlement math), the funding model, typed
// errors, and repository ports. Pure Go — no gorm/gin/redis/grpc.
package domain

import "time"

// Sides and statuses.
const (
	SideLong  = "LONG"
	SideShort = "SHORT"

	StatusOpen       = "OPEN"
	StatusClosed     = "CLOSED"
	StatusLiquidated = "LIQUIDATED"
)

// Economic constants.
const (
	FeeRate               = 0.0005 // 0.05% taker fee per side
	MaintenanceMarginRate = 0.005  // 0.5% maintenance margin
	MaxLeverage           = 125
)

// Position is a leveraged perpetual position aggregate.
type Position struct {
	ID               uint
	UserID           uint
	Pair             string
	Side             string
	Leverage         int
	EntryPrice       float64
	MarkPrice        float64
	Size             float64
	Margin           float64
	UnrealizedPnL    float64
	LiquidationPrice float64
	TakeProfit       float64
	StopLoss         float64
	Status           string
	CreatedAt        time.Time
	ClosedAt         *time.Time
}

// Settlement describes the wallet effect of closing a position: unlock the
// reserved margin, then apply NetDelta (signed) to the available balance.
type Settlement struct {
	UnlockMargin float64
	NetDelta     float64
}

// OpenPosition validates inputs and builds an OPEN position, computing the
// margin and liquidation price from the entry mark price.
func OpenPosition(userID uint, pair, side string, leverage int, size, mark, takeProfit, stopLoss float64) (*Position, error) {
	if side != SideLong && side != SideShort {
		return nil, ErrInvalidSide
	}
	if leverage < 1 || leverage > MaxLeverage {
		return nil, ErrInvalidLeverage
	}
	if size <= 0 {
		return nil, ErrInvalidSize
	}
	if mark <= 0 {
		return nil, ErrPriceUnavailable
	}
	notional := size * mark
	return &Position{
		UserID: userID, Pair: pair, Side: side, Leverage: leverage,
		EntryPrice: mark, MarkPrice: mark, Size: size,
		Margin:           notional / float64(leverage),
		LiquidationPrice: CalcLiquidationPrice(side, mark, leverage),
		TakeProfit:       takeProfit, StopLoss: stopLoss, Status: StatusOpen,
	}, nil
}

// CalcUnrealizedPnL is the mark-to-market P&L at the given price.
func (p *Position) CalcUnrealizedPnL(mark float64) float64 {
	if p.Side == SideLong {
		return p.Size * (mark - p.EntryPrice)
	}
	return p.Size * (p.EntryPrice - mark)
}

func (p *Position) Notional(price float64) float64 { return p.Size * price }
func (p *Position) OpenFee() float64                { return p.Size * p.EntryPrice * FeeRate }
func (p *Position) CloseFee(mark float64) float64   { return p.Size * mark * FeeRate }

// MaintenanceMargin is the equity floor below which the position is force-closed.
func (p *Position) MaintenanceMargin(mark float64) float64 {
	return p.Size * mark * MaintenanceMarginRate
}

// ShouldLiquidate reports whether mark has crossed the liquidation price.
func (p *Position) ShouldLiquidate(mark float64) bool {
	if p.Side == SideLong {
		return mark <= p.LiquidationPrice
	}
	return mark >= p.LiquidationPrice
}

// ShouldTriggerTPSL reports whether a take-profit or stop-loss has been hit.
func (p *Position) ShouldTriggerTPSL(mark float64) bool {
	if p.Side == SideLong {
		return (p.TakeProfit > 0 && mark >= p.TakeProfit) || (p.StopLoss > 0 && mark <= p.StopLoss)
	}
	return (p.TakeProfit > 0 && mark <= p.TakeProfit) || (p.StopLoss > 0 && mark >= p.StopLoss)
}

// Close performs a user/admin close at mark: realized PnL is net of both the
// open and close fees; the wallet net delta is PnL minus close fee, capped so a
// close can never push the user negative.
func (p *Position) Close(mark float64, at time.Time) Settlement {
	pnl := p.CalcUnrealizedPnL(mark)
	closeFee := p.CloseFee(mark)
	net := pnl - closeFee
	if net < -p.Margin {
		net = -p.Margin
	}
	p.finalize(StatusClosed, mark, at, pnl-closeFee-p.OpenFee())
	return Settlement{UnlockMargin: p.Margin, NetDelta: net}
}

// ForceCloseTPSL closes on a TP/SL trigger: no close fee (courtesy), loss capped
// at margin, realized net of the open fee.
func (p *Position) ForceCloseTPSL(mark float64, at time.Time) Settlement {
	pnl := p.CalcUnrealizedPnL(mark)
	if pnl < -p.Margin {
		pnl = -p.Margin
	}
	p.finalize(StatusClosed, mark, at, pnl-p.OpenFee())
	return Settlement{UnlockMargin: p.Margin, NetDelta: pnl}
}

// MarginCallClose force-closes during a portfolio margin call with a
// pre-computed pnl, marking the position LIQUIDATED.
func (p *Position) MarginCallClose(mark, pnl float64, at time.Time) Settlement {
	if pnl < -p.Margin {
		pnl = -p.Margin
	}
	p.finalize(StatusLiquidated, mark, at, pnl-p.OpenFee())
	return Settlement{UnlockMargin: p.Margin, NetDelta: pnl}
}

// Liquidate is a full-margin wipeout when the liquidation price is breached.
func (p *Position) Liquidate(mark float64, at time.Time) Settlement {
	p.finalize(StatusLiquidated, mark, at, -p.Margin)
	return Settlement{UnlockMargin: p.Margin, NetDelta: -p.Margin}
}

func (p *Position) finalize(status string, mark float64, at time.Time, realized float64) {
	p.Status = status
	p.MarkPrice = mark
	closed := at
	p.ClosedAt = &closed
	p.UnrealizedPnL = realized
}

// CalcLiquidationPrice is the price at which the position is liquidated,
// including the maintenance-margin buffer.
func CalcLiquidationPrice(side string, entryPrice float64, leverage int) float64 {
	lev := float64(leverage)
	if side == SideLong {
		return entryPrice * (1 - 1/lev + MaintenanceMarginRate)
	}
	return entryPrice * (1 + 1/lev - MaintenanceMarginRate)
}
