package model

import "time"

// UserTradePair tracks trade frequency between two users for fraud detection.
// Key: sorted (user1 < user2) + pair. Updated atomically on each trade.
type UserTradePair struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	User1ID    uint      `gorm:"not null;uniqueIndex:idx_utp_users_pair" json:"user1Id"`
	User2ID    uint      `gorm:"not null;uniqueIndex:idx_utp_users_pair" json:"user2Id"`
	Pair       string    `gorm:"not null;uniqueIndex:idx_utp_users_pair" json:"pair"`
	TradeCount int       `gorm:"default:0" json:"tradeCount"`
	TotalVol   float64   `gorm:"type:decimal(30,2);default:0" json:"totalVol"`
	FirstTrade time.Time `json:"firstTrade"`
	LastTrade  time.Time `json:"lastTrade"`
}

// SortedUserIDs returns (smaller, larger) to ensure consistent key
func SortedUserIDs(a, b uint) (uint, uint) {
	if a < b {
		return a, b
	}
	return b, a
}
