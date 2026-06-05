package domain

import "time"

// Trade is an executed match between a buy and a sell order.
type Trade struct {
	ID          uint
	Pair        string
	BuyOrderID  uint
	SellOrderID uint
	BuyerID     uint
	SellerID    uint
	Price       float64
	Amount      float64
	Total       float64 // Price * Amount (quote)
	BuyerFee    float64
	SellerFee   float64
	CreatedAt   time.Time
}
