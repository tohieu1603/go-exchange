package model

import "time"

type Order struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"not null;index:idx_order_user_status" json:"userId"`
	Pair         string    `gorm:"not null;index:idx_order_pair_status" json:"pair"`
	Side         string    `gorm:"not null" json:"side"`
	Type         string    `gorm:"not null" json:"type"`
	Price        float64   `gorm:"type:decimal(30,10)" json:"price"`
	StopPrice    float64   `gorm:"type:decimal(30,10)" json:"stopPrice"`
	Amount       float64   `gorm:"type:decimal(30,10);not null" json:"amount"`
	FilledAmount float64   `gorm:"type:decimal(30,10);default:0" json:"filledAmount"`
	Status       string    `gorm:"default:OPEN;index:idx_order_user_status;index:idx_order_pair_status" json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (o *Order) Remaining() float64 {
	return o.Amount - o.FilledAmount
}

func (o *Order) IsFilled() bool {
	return o.FilledAmount >= o.Amount
}
