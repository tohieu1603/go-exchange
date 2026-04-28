package model

import "time"

type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;index:idx_notif_user" json:"userId"`
	Type      string    `gorm:"not null" json:"type"` // ORDER_FILLED, POSITION_OPENED, POSITION_CLOSED, POSITION_LIQUIDATED, DEPOSIT_CONFIRMED, MARGIN_CALL
	Title     string    `gorm:"not null" json:"title"`
	Message   string    `gorm:"not null" json:"message"`
	Pair      string    `json:"pair,omitempty"`
	IsRead    bool      `gorm:"default:false;index:idx_notif_user" json:"isRead"`
	CreatedAt time.Time `json:"createdAt"`
}
