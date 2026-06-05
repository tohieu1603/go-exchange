// Package domain holds the spot-trading domain: the Order and Trade entities,
// the in-memory matching order book, typed errors, and repository ports. Pure
// Go — no gorm/gin/redis/grpc.
package domain

import "time"

// Sides, types and statuses.
const (
	SideBuy  = "BUY"
	SideSell = "SELL"

	TypeMarket    = "MARKET"
	TypeLimit     = "LIMIT"
	TypeStopLimit = "STOP_LIMIT"

	StatusOpen      = "OPEN"
	StatusPartial   = "PARTIAL"
	StatusFilled    = "FILLED"
	StatusCancelled = "CANCELLED"
)

// Order is a spot order aggregate.
type Order struct {
	ID           uint
	UserID       uint
	Pair         string
	Side         string
	Type         string
	Price        float64
	StopPrice    float64
	Amount       float64
	FilledAmount float64
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Remaining is the unfilled quantity.
func (o *Order) Remaining() float64 { return o.Amount - o.FilledAmount }

// IsFilled reports whether the order is fully matched.
func (o *Order) IsFilled() bool { return o.FilledAmount >= o.Amount }

// IsCancellable reports whether the order can still be cancelled.
func (o *Order) IsCancellable() bool { return o.Status == StatusOpen || o.Status == StatusPartial }
