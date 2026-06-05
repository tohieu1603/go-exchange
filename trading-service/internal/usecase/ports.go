// Package usecase holds the trading application logic: order placement/cancel,
// the matching engine, and CQRS projectors. Depends on domain + the outbound
// ports here (plus shared/eventbus for the publish surface).
package usecase

import "context"

// BalanceCache is the Redis hot-path balance store (atomic Lua ops).
type BalanceCache interface {
	Deduct(ctx context.Context, userID uint, currency string, amount float64) (float64, error)
	Credit(ctx context.Context, userID uint, currency string, amount float64) (float64, error)
	Lock(ctx context.Context, userID uint, currency string, amount float64) error
	Unlock(ctx context.Context, userID uint, currency string, amount float64) error
}

// WalletGateway pre-checks/warms a user's balance via wallet-service.
type WalletGateway interface {
	CheckBalance(ctx context.Context, userID uint, currency string, needed float64) error
}

// PriceGateway reads the live market price for a pair.
type PriceGateway interface {
	GetPrice(ctx context.Context, pair string) (float64, error)
}

// FeeResolver returns maker/taker fee rates for a user.
type FeeResolver interface {
	Rates(userID uint) (maker, taker float64)
}

// FlatFeeResolver is a fixed-rate fallback.
type FlatFeeResolver struct {
	Maker float64
	Taker float64
}

func NewFlatFeeResolver(maker, taker float64) *FlatFeeResolver {
	return &FlatFeeResolver{Maker: maker, Taker: taker}
}

func (r *FlatFeeResolver) Rates(uint) (float64, float64) { return r.Maker, r.Taker }
