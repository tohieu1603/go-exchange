package domain

import "context"

// OrderRepository persists spot orders.
type OrderRepository interface {
	Create(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id uint) (*Order, error)
	FindByUserAndID(ctx context.Context, userID, orderID uint) (*Order, error)
	UpdateStatus(ctx context.Context, id uint, status string, filledAmount, price float64) error
	FindOpen(ctx context.Context, userID uint) ([]Order, error)
	FindPaginated(ctx context.Context, userID uint, status string, page, size int) ([]Order, int64, error)
	Save(ctx context.Context, order *Order) error
	FindOpenLimitOrders(ctx context.Context) ([]Order, error)
}

// TradeRepository persists executed trades (CQRS projection of trade.executed).
type TradeRepository interface {
	Create(ctx context.Context, t *Trade) error
}
