package model

import "time"

type Withdrawal struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"userId"`
	Amount      float64   `gorm:"type:decimal(30,2);not null" json:"amount"`
	Currency    string    `gorm:"default:VND" json:"currency"`
	BankCode    string    `gorm:"not null" json:"bankCode"`
	BankAccount string    `gorm:"not null" json:"bankAccount"`
	AccountName string    `gorm:"not null" json:"accountName"`
	Status      string    `gorm:"default:PENDING;index" json:"status"` // PENDING, APPROVED, REJECTED
	AdminNote   string    `json:"adminNote"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
