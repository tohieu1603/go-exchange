package model

import "time"

// FeeTier defines maker/taker fee bracket based on 30-day rolling trading volume.
// Volume measured in USDT-equivalent total (price * amount).
type FeeTier struct {
	ID          uint    `gorm:"primaryKey" json:"id"`
	Level       int     `gorm:"uniqueIndex;not null" json:"level"`         // VIP0, VIP1, ...
	Name        string  `gorm:"not null" json:"name"`                       // "VIP0"
	MinVolume   float64 `gorm:"type:decimal(30,2);not null" json:"minVolume"` // 30-day USDT
	MakerFee    float64 `gorm:"type:decimal(8,6);not null" json:"makerFee"`   // 0.001 = 0.1%
	TakerFee    float64 `gorm:"type:decimal(8,6);not null" json:"takerFee"`
	Description string  `json:"description"`
}

// UserVolume30d caches a user's rolling 30-day USDT volume for fast tier lookup.
// Refreshed by a periodic job consuming trade.executed events.
type UserVolume30d struct {
	UserID    uint      `gorm:"primaryKey" json:"userId"`
	Volume    float64   `gorm:"type:decimal(30,2);not null;default:0" json:"volume"`
	TierLevel int       `gorm:"default:0" json:"tierLevel"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DefaultFeeTiers - reasonable starter tiers (mirrors Binance VIP0–VIP3 spirit at smaller scale)
var DefaultFeeTiers = []FeeTier{
	{Level: 0, Name: "VIP0", MinVolume: 0,         MakerFee: 0.0010, TakerFee: 0.0010, Description: "Default tier"},
	{Level: 1, Name: "VIP1", MinVolume: 50_000,     MakerFee: 0.0009, TakerFee: 0.0010, Description: "30-day volume ≥ 50K USDT"},
	{Level: 2, Name: "VIP2", MinVolume: 500_000,    MakerFee: 0.0008, TakerFee: 0.0009, Description: "30-day volume ≥ 500K USDT"},
	{Level: 3, Name: "VIP3", MinVolume: 5_000_000,  MakerFee: 0.0006, TakerFee: 0.0008, Description: "30-day volume ≥ 5M USDT"},
	{Level: 4, Name: "VIP4", MinVolume: 50_000_000, MakerFee: 0.0004, TakerFee: 0.0006, Description: "30-day volume ≥ 50M USDT"},
}
