package usecase

import "time"

// TradeDoc / OrderDoc / … are the search-index projections of domain events.

type TradeDoc struct {
	Pair      string    `json:"pair"`
	BuyerID   uint      `json:"buyerId"`
	SellerID  uint      `json:"sellerId"`
	Price     float64   `json:"price"`
	Amount    float64   `json:"amount"`
	Total     float64   `json:"total"`
	Side      string    `json:"side"`
	CreatedAt time.Time `json:"createdAt"`
}

type OrderDoc struct {
	OrderID      uint      `json:"orderId"`
	UserID       uint      `json:"userId"`
	Pair         string    `json:"pair"`
	Side         string    `json:"side"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Price        float64   `json:"price"`
	Amount       float64   `json:"amount"`
	FilledAmount float64   `json:"filledAmount"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type BalanceDoc struct {
	UserID    uint      `json:"userId"`
	Currency  string    `json:"currency"`
	Delta     float64   `json:"delta"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

type NotificationDoc struct {
	UserID    uint      `json:"userId"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Pair      string    `json:"pair"`
	CreatedAt time.Time `json:"createdAt"`
}

// AuditDoc shape for the audit_logs ES index. Timestamp is the canonical
// @timestamp field Kibana uses as the data-view time field.
type AuditDoc struct {
	UserID    uint      `json:"userId"`
	Email     string    `json:"email,omitempty"`
	Action    string    `json:"action"`
	Outcome   string    `json:"outcome"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
	DeviceID  string    `json:"deviceId,omitempty"`
	NewDevice bool      `json:"newDevice,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Timestamp time.Time `json:"@timestamp"`
}
