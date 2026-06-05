package domain

import "time"

// BonusPromotion defines a bonus campaign created by admin
type BonusPromotion struct {
	ID             uint      `json:"id"`
	Name           string    `json:"name"`                                   // "Welcome 100% Bonus"
	Description    string    `json:"description"`
	BonusPercent   float64   `json:"bonusPercent"`          // 10-100%
	MaxBonusAmount float64   `json:"maxBonusAmount"`                // max bonus per user (USD)
	TargetType     string    `json:"targetType"`                             // ALL, SPECIFIC_USERS
	TargetUserIDs  string    `json:"targetUserIds,omitempty"`                                // comma-separated user IDs (for SPECIFIC_USERS)
	TriggerType    string    `json:"triggerType"`                            // ON_DEPOSIT, MANUAL
	MinDeposit     float64   `json:"minDeposit"`                   // minimum deposit to qualify
	IsActive       bool      `json:"isActive"`
	StartAt        time.Time `json:"startAt"`
	EndAt          time.Time `json:"endAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

// UserBonus tracks bonus credited to a user from a promotion
type UserBonus struct {
	ID          uint       `json:"id"`
	UserID      uint       `json:"userId"`
	PromotionID uint       `json:"promotionId"`
	DepositID   uint       `json:"depositId,omitempty"`                                    // linked deposit (for ON_DEPOSIT)
	BonusAmount float64    `json:"bonusAmount"`
	UsedAmount  float64    `json:"usedAmount"`         // consumed in trading
	Status      string     `json:"status"`                          // ACTIVE, USED, EXPIRED, REVOKED
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// RemainingBonus returns unused bonus amount
func (b *UserBonus) RemainingBonus() float64 {
	return b.BonusAmount - b.UsedAmount
}
