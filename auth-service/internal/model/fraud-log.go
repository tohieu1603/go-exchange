package model

import "time"

// FraudLog tracks suspicious activity detected by anti-fraud system
type FraudLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserIDs     string    `gorm:"not null" json:"userIds"`        // comma-separated involved user IDs
	FraudType   string    `gorm:"not null" json:"fraudType"`      // BONUS_FARMING, WASH_TRADING, MULTI_ACCOUNT
	Description string    `json:"description"`
	Evidence    string    `gorm:"type:text" json:"evidence"`      // JSON evidence data
	Action      string    `gorm:"default:FLAGGED" json:"action"`  // FLAGGED, ACCOUNTS_LOCKED, BONUS_REVOKED, DISMISSED
	AdminNote   string    `json:"adminNote,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
