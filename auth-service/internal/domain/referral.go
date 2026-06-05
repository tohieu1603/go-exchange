package domain

import "time"

// ReferralCode is the public code (e.g. "MX-A1B2C3") a user shares to invite others.
// Each user has exactly one default code; optional custom codes for campaigns.
type ReferralCode struct {
	ID         uint      `json:"id"`
	UserID     uint      `json:"userId"`
	Code       string    `json:"code"`
	IsDefault  bool      `json:"isDefault"`
	UsageCount int       `json:"usageCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Referral records the binding referrer → referee created at registration.
// Immutable: a user can only ever be referred by one referrer.
type Referral struct {
	ID         uint      `json:"id"`
	ReferrerID uint      `json:"referrerId"`
	RefereeID  uint      `json:"refereeId"` // referee can only be referred once
	Code       string    `json:"code"`                   // code used at signup
	Tier       int       `json:"tier"`                   // 1 = direct, 2 = indirect (future)
	CreatedAt  time.Time `json:"createdAt"`
}

// ReferralCommission tracks each commission credited to a referrer when their
// referee makes a trade. RefID = trade ID for idempotency.
type ReferralCommission struct {
	ID         uint      `json:"id"`
	ReferrerID uint      `json:"referrerId"`
	RefereeID  uint      `json:"refereeId"`
	TradeID    uint      `json:"tradeId"`
	Currency   string    `json:"currency"`                // settled in fee currency (USDT)
	FeeAmount  float64   `json:"feeAmount"`
	Rate       float64   `json:"rate"`   // e.g. 0.20 = 20% of fee
	Commission float64   `json:"commission"`
	CreatedAt  time.Time `json:"createdAt"`
}
