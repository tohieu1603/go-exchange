package domain

import "context"

// PositionRepository persists futures positions. The *ForUpdate reads run inside
// a transaction (carried on ctx by the application TxManager) and take a
// SELECT ... FOR UPDATE row lock to serialise close/liquidate.
type PositionRepository interface {
	Create(ctx context.Context, pos *Position) error
	FindOpenByUser(ctx context.Context, userID uint) ([]Position, error)
	FindByUserAndStatus(ctx context.Context, userID uint, status string) ([]Position, error)
	FindAllOpen(ctx context.Context) ([]Position, error)
	FindByIDForUpdate(ctx context.Context, id uint, status string) (*Position, error)
	FindByUserAndIDForUpdate(ctx context.Context, userID, id uint, status string) (*Position, error)
	Save(ctx context.Context, pos *Position) error
	UpdateTPSL(ctx context.Context, id, userID uint, takeProfit, stopLoss *float64) error
	FindByUserAndID(ctx context.Context, userID, id uint, status string) (*Position, error)
}

// FundingRepository persists funding rates and per-position payments.
type FundingRepository interface {
	CreateRate(ctx context.Context, r *FundingRate) error
	LatestRate(ctx context.Context, pair string) (*FundingRate, error)
	RecentRates(ctx context.Context, pair string, limit int) ([]FundingRate, error)
	CreatePayment(ctx context.Context, p *FundingPayment) error
	HistoryByUser(ctx context.Context, userID uint, page, size int) ([]FundingPayment, int64, error)
}
