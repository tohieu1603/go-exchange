package model

import "time"

// FundingRate snapshot for a perpetual futures pair at a settlement interval.
// Standard interval: every 8 hours (00:00, 08:00, 16:00 UTC).
// Sign convention: positive rate → LONG pays SHORT (premium); negative → SHORT pays LONG.
type FundingRate struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Pair       string    `gorm:"not null;index:idx_fr_pair_time,unique" json:"pair"`
	Rate       float64   `gorm:"type:decimal(10,8);not null" json:"rate"` // e.g. 0.0001 = 0.01%
	IndexPrice float64   `gorm:"type:decimal(30,10)" json:"indexPrice"`
	MarkPrice  float64   `gorm:"type:decimal(30,10)" json:"markPrice"`
	Interval   string    `gorm:"default:'8h'" json:"interval"`
	SettledAt  time.Time `gorm:"not null;index:idx_fr_pair_time,unique" json:"settledAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

// FundingPayment records each funding fee charged to / credited to a position.
// Idempotent on (positionId, fundingRateId).
type FundingPayment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	PositionID    uint      `gorm:"not null;uniqueIndex:idx_fp_pos_rate" json:"positionId"`
	UserID        uint      `gorm:"not null;index" json:"userId"`
	FundingRateID uint      `gorm:"not null;uniqueIndex:idx_fp_pos_rate" json:"fundingRateId"`
	Pair          string    `gorm:"not null" json:"pair"`
	Side          string    `gorm:"not null" json:"side"`                                  // LONG or SHORT
	Notional      float64   `gorm:"type:decimal(30,10);not null" json:"notional"`           // size * markPrice
	Rate          float64   `gorm:"type:decimal(10,8);not null" json:"rate"`
	Amount        float64   `gorm:"type:decimal(30,10);not null" json:"amount"`             // signed: negative = paid by user
	CreatedAt     time.Time `json:"createdAt"`
}
