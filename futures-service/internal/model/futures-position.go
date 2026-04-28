package model

import "time"

type FuturesPosition struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	UserID           uint       `gorm:"not null;index" json:"userId"`
	Pair             string     `gorm:"not null;index" json:"pair"`
	Side             string     `gorm:"not null" json:"side"`                    // LONG, SHORT
	Leverage         int        `gorm:"not null" json:"leverage"`                // 1-125
	EntryPrice       float64    `gorm:"type:decimal(30,10)" json:"entryPrice"`
	MarkPrice        float64    `gorm:"type:decimal(30,10)" json:"markPrice"`
	Size             float64    `gorm:"type:decimal(30,10)" json:"size"`          // base currency amount
	Margin           float64    `gorm:"type:decimal(30,2)" json:"margin"`         // VND collateral
	UnrealizedPnL    float64    `gorm:"type:decimal(30,2)" json:"unrealizedPnl"`
	LiquidationPrice float64    `gorm:"type:decimal(30,10)" json:"liquidationPrice"`
	TakeProfit       float64    `gorm:"type:decimal(30,10)" json:"takeProfit"`
	StopLoss         float64    `gorm:"type:decimal(30,10)" json:"stopLoss"`
	Status           string     `gorm:"default:OPEN;index" json:"status"`         // OPEN, CLOSED, LIQUIDATED
	CreatedAt        time.Time  `json:"createdAt"`
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
}

// CalcUnrealizedPnL returns current unrealized PnL based on mark price.
func (p *FuturesPosition) CalcUnrealizedPnL(markPrice float64) float64 {
	if p.Side == "LONG" {
		return p.Size * (markPrice - p.EntryPrice)
	}
	return p.Size * (p.EntryPrice - markPrice) // SHORT
}

// CalcLiquidationPrice returns the price at which position gets liquidated.
func CalcLiquidationPrice(side string, entryPrice float64, leverage int) float64 {
	lev := float64(leverage)
	if side == "LONG" {
		return entryPrice * (1 - 1/lev + 0.005) // 0.5% maintenance margin
	}
	return entryPrice * (1 + 1/lev - 0.005)
}
