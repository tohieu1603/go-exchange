package eventbus

// Domain events - immutable facts that happened in the system

// TradeEvent is published when a trade is executed (orderbook match or instant fill).
type TradeEvent struct {
	Pair        string  `json:"pair"`
	BuyOrderID  uint    `json:"buyOrderId"`
	SellOrderID uint    `json:"sellOrderId"`
	BuyerID     uint    `json:"buyerId"`
	SellerID    uint    `json:"sellerId"`
	Price       float64 `json:"price"`
	Amount      float64 `json:"amount"`
	Total       float64 `json:"total"`
	BuyerFee    float64 `json:"buyerFee"`
	SellerFee   float64 `json:"sellerFee"`
	Side        string  `json:"side"` // taker side
}

// OrderEvent is published when order status changes.
type OrderEvent struct {
	OrderID      uint    `json:"orderId"`
	UserID       uint    `json:"userId"`
	Pair         string  `json:"pair"`
	Side         string  `json:"side"`
	Type         string  `json:"type"`
	Price        float64 `json:"price"`
	Amount       float64 `json:"amount"`
	FilledAmount float64 `json:"filledAmount"`
	Status       string  `json:"status"` // OPEN, PARTIAL, FILLED, CANCELLED
}

// BalanceEvent is published when a wallet balance changes.
type BalanceEvent struct {
	UserID   uint    `json:"userId"`
	Currency string  `json:"currency"`
	Delta    float64 `json:"delta"`  // amount changed (+ or -)
	Reason   string  `json:"reason"` // trade, deposit, withdraw, fee, liquidation
	RefID    string  `json:"refId"`  // reference (trade ID, order ID, etc.)
}

// NotificationEvent is published when a notification should be created.
type NotificationEvent struct {
	UserID  uint   `json:"userId"`
	Type    string `json:"type"` // POSITION_OPENED, POSITION_CLOSED, POSITION_LIQUIDATED, MARGIN_CALL, DEPOSIT_CONFIRMED
	Title   string `json:"title"`
	Message string `json:"message"`
	Pair    string `json:"pair"`
}

// UserRegisteredEvent is published when a new user registers.
// Wallet-service consumes this to create wallets in its own DB.
type UserRegisteredEvent struct {
	UserID       uint   `json:"userId"`
	Email        string `json:"email"`
	FullName     string `json:"fullName"`
	Role         string `json:"role,omitempty"`         // USER, ADMIN, SYSTEM
	SeedProfile  string `json:"seedProfile,omitempty"`  // optional: "default" | "admin" | "system" | "demo"
	ReferralCode string `json:"referralCode,omitempty"` // code presented at signup
	ClientIP     string `json:"clientIp,omitempty"`
}

// AuditRequestEvent is published by non-auth services (wallet, futures,
// trading, …) when they want to persist an audit log row. Auth-service
// owns the audit_logs table and is the sole consumer of this topic.
type AuditRequestEvent struct {
	UserID  uint   `json:"userId"`            // subject of the action (the affected end-user)
	Email   string `json:"email,omitempty"`   // optional snapshot for searchability
	Action  string `json:"action"`            // dotted (eg "admin.order.cancel", "admin.balance.adjust")
	Outcome string `json:"outcome"`           // "success" | "failure"
	IP      string `json:"ip,omitempty"`      // admin's source IP
	Detail  string `json:"detail,omitempty"`  // freeform JSON or human-readable context
}

// AuditLogEvent mirrors a persisted audit row, published to event bus so
// es-indexer can fan-out to Elasticsearch for Kibana dashboards.
type AuditLogEvent struct {
	UserID    uint   `json:"userId"`
	Email     string `json:"email,omitempty"`
	Action    string `json:"action"`
	Outcome   string `json:"outcome"`
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
	NewDevice bool   `json:"newDevice,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Timestamp int64  `json:"timestamp"` // unix ms — required by Kibana time index
}

// PositionEvent is published when a futures position changes.
type PositionEvent struct {
	PositionID uint    `json:"positionId"`
	UserID     uint    `json:"userId"`
	Pair       string  `json:"pair"`
	Side       string  `json:"side"`
	Status     string  `json:"status"` // OPEN, CLOSED, LIQUIDATED
	EntryPrice float64 `json:"entryPrice"`
	MarkPrice  float64 `json:"markPrice"`
	Size       float64 `json:"size"`
	Margin     float64 `json:"margin"`
	PnL        float64 `json:"pnl"`
}
