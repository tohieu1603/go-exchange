package model

import "time"

type Trade struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Pair        string    `gorm:"not null;index" json:"pair"`
	BuyOrderID  uint      `gorm:"not null" json:"buyOrderId"`
	SellOrderID uint      `gorm:"not null" json:"sellOrderId"`
	BuyerID     uint      `gorm:"not null" json:"buyerId"`
	SellerID    uint      `gorm:"not null" json:"sellerId"`
	Price       float64   `gorm:"type:decimal(30,10);not null" json:"price"`
	Amount      float64   `gorm:"type:decimal(30,10);not null" json:"amount"`
	Total       float64   `gorm:"type:decimal(30,2);not null" json:"total"` // Price * Amount (in VND)
	BuyerFee    float64   `gorm:"type:decimal(30,10);default:0" json:"buyerFee"`
	SellerFee   float64   `gorm:"type:decimal(30,10);default:0" json:"sellerFee"`
	CreatedAt   time.Time `json:"createdAt"`
}
