package domain

import "time"

// UserTradePair tracks trade frequency between two users for fraud detection.
// Key: sorted (user1 < user2) + pair. Updated atomically on each trade.
type UserTradePair struct {
	ID         uint      `json:"id"`
	User1ID    uint      `json:"user1Id"`
	User2ID    uint      `json:"user2Id"`
	Pair       string    `json:"pair"`
	TradeCount int       `json:"tradeCount"`
	TotalVol   float64   `json:"totalVol"`
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
