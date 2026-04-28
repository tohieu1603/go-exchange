package model

import "time"

// BonusPromotion defines a bonus campaign created by admin
type BonusPromotion struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"not null" json:"name"`                                   // "Welcome 100% Bonus"
	Description    string    `json:"description"`
	BonusPercent   float64   `gorm:"type:decimal(5,2);not null" json:"bonusPercent"`          // 10-100%
	MaxBonusAmount float64   `gorm:"type:decimal(20,2)" json:"maxBonusAmount"`                // max bonus per user (USD)
	TargetType     string    `gorm:"not null" json:"targetType"`                             // ALL, SPECIFIC_USERS
	TargetUserIDs  string    `json:"targetUserIds,omitempty"`                                // comma-separated user IDs (for SPECIFIC_USERS)
	TriggerType    string    `gorm:"not null" json:"triggerType"`                            // ON_DEPOSIT, MANUAL
	MinDeposit     float64   `gorm:"type:decimal(20,2)" json:"minDeposit"`                   // minimum deposit to qualify
	IsActive       bool      `gorm:"default:true" json:"isActive"`
	StartAt        time.Time `json:"startAt"`
	EndAt          time.Time `json:"endAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

// UserBonus tracks bonus credited to a user from a promotion
type UserBonus struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"index;not null" json:"userId"`
	PromotionID uint       `gorm:"index;not null" json:"promotionId"`
	DepositID   uint       `json:"depositId,omitempty"`                                    // linked deposit (for ON_DEPOSIT)
	BonusAmount float64    `gorm:"type:decimal(20,2);not null" json:"bonusAmount"`
	UsedAmount  float64    `gorm:"type:decimal(20,2);default:0" json:"usedAmount"`         // consumed in trading
	Status      string     `gorm:"default:ACTIVE" json:"status"`                          // ACTIVE, USED, EXPIRED, REVOKED
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// RemainingBonus returns unused bonus amount
func (b *UserBonus) RemainingBonus() float64 {
	return b.BonusAmount - b.UsedAmount
}
