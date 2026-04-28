package model

import "time"

type Deposit struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"not null;index" json:"userId"`
	Amount       float64   `gorm:"type:decimal(30,2);not null" json:"amount"`          // VND amount
	AmountUSDT   float64   `gorm:"type:decimal(30,8)" json:"amountUsdt"`               // USDT received
	ExchangeRate float64   `gorm:"type:decimal(20,2)" json:"exchangeRate"`             // VND per USDT
	Currency     string    `gorm:"default:VND" json:"currency"`
	Method       string    `gorm:"default:BANK_TRANSFER" json:"method"`
	Status       string    `gorm:"default:PENDING" json:"status"` // PENDING, CONFIRMED, FAILED
	OrderCode    string    `gorm:"uniqueIndex" json:"orderCode"`
	QRCodeURL    string    `json:"qrCodeUrl"`
	SepayRef     string    `json:"sepayRef"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
