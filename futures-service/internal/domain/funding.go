package domain

import "time"

// MaxFundingRate caps the per-interval funding rate at ±0.5%.
const MaxFundingRate = 0.005

// FundingRate is a settled funding snapshot for a perpetual pair.
// Sign convention: positive rate → LONG pays SHORT.
type FundingRate struct {
	ID         uint
	Pair       string
	Rate       float64
	IndexPrice float64
	MarkPrice  float64
	Interval   string
	SettledAt  time.Time
	CreatedAt  time.Time
}

// FundingPayment is a per-position funding charge/credit (idempotent on
// position+rate).
type FundingPayment struct {
	ID            uint
	PositionID    uint
	UserID        uint
	FundingRateID uint
	Pair          string
	Side          string
	Notional      float64
	Rate          float64
	Amount        float64 // signed: negative = paid by user
	CreatedAt     time.Time
}

// CalcFundingRate is the premium of mark over index, capped at ±MaxFundingRate.
func CalcFundingRate(mark, index float64) float64 {
	if index <= 0 {
		return 0
	}
	rate := (mark - index) / index
	if rate > MaxFundingRate {
		return MaxFundingRate
	}
	if rate < -MaxFundingRate {
		return -MaxFundingRate
	}
	return rate
}

// FundingAmount is the signed wallet delta for a position at settlement.
// LONG pays (negative) on a positive rate; SHORT receives.
func FundingAmount(side string, rate, notional float64) float64 {
	if side == SideLong {
		return -rate * notional
	}
	return rate * notional
}
