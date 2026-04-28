package model

import "time"

type Wallet struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"not null;index:idx_user_currency,unique" json:"userId"`
	Currency      string    `gorm:"not null;index:idx_user_currency,unique" json:"currency"` // VND, BTC, ETH, etc.
	Balance       float64   `gorm:"type:decimal(30,10);default:0" json:"balance"`
	LockedBalance float64   `gorm:"type:decimal(30,10);default:0" json:"lockedBalance"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (w *Wallet) Available() float64 {
	return w.Balance - w.LockedBalance
}
