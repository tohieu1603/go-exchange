package domain

import "time"

// FraudLog tracks suspicious activity detected by anti-fraud system
type FraudLog struct {
	ID          uint      `json:"id"`
	UserIDs     string    `json:"userIds"`        // comma-separated involved user IDs
	FraudType   string    `json:"fraudType"`      // BONUS_FARMING, WASH_TRADING, MULTI_ACCOUNT
	Description string    `json:"description"`
	Evidence    string    `json:"evidence"`      // JSON evidence data
	Action      string    `json:"action"`  // FLAGGED, ACCOUNTS_LOCKED, BONUS_REVOKED, DISMISSED
	AdminNote   string    `json:"adminNote,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
