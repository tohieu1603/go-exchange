package model

import "time"

// ReferralCode is the public code (e.g. "MX-A1B2C3") a user shares to invite others.
// Each user has exactly one default code; optional custom codes for campaigns.
type ReferralCode struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"not null;index" json:"userId"`
	Code       string    `gorm:"uniqueIndex;not null" json:"code"`
	IsDefault  bool      `gorm:"default:true" json:"isDefault"`
	UsageCount int       `gorm:"default:0" json:"usageCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Referral records the binding referrer → referee created at registration.
// Immutable: a user can only ever be referred by one referrer.
type Referral struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ReferrerID uint      `gorm:"not null;index" json:"referrerId"`
	RefereeID  uint      `gorm:"uniqueIndex;not null" json:"refereeId"` // referee can only be referred once
	Code       string    `gorm:"not null" json:"code"`                   // code used at signup
	Tier       int       `gorm:"default:1" json:"tier"`                   // 1 = direct, 2 = indirect (future)
	CreatedAt  time.Time `json:"createdAt"`
}

// ReferralCommission tracks each commission credited to a referrer when their
// referee makes a trade. RefID = trade ID for idempotency.
type ReferralCommission struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ReferrerID uint      `gorm:"not null;index" json:"referrerId"`
	RefereeID  uint      `gorm:"not null;index" json:"refereeId"`
	TradeID    uint      `gorm:"not null;uniqueIndex:idx_ref_trade" json:"tradeId"`
	Currency   string    `gorm:"not null" json:"currency"`                // settled in fee currency (USDT)
	FeeAmount  float64   `gorm:"type:decimal(30,10);not null" json:"feeAmount"`
	Rate       float64   `gorm:"type:decimal(6,4);not null" json:"rate"`   // e.g. 0.20 = 20% of fee
	Commission float64   `gorm:"type:decimal(30,10);not null" json:"commission"`
	CreatedAt  time.Time `json:"createdAt"`
}
