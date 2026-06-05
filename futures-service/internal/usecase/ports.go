// Package usecase holds the futures application logic: open/close/TP-SL
// positions, the liquidation engine, and funding settlement. Depends on domain +
// the outbound ports here — never on pgx/gin/grpc directly.
package usecase

import "context"

// WalletGateway is the outbound port to wallet-service (implemented by the gRPC
// client). Margin is Lock'd (not deducted) as collateral; fees/PnL are real
// Deduct/Credit movements.
type WalletGateway interface {
	CheckBalance(ctx context.Context, userID uint, currency string, needed float64) error
	Deduct(ctx context.Context, userID uint, currency string, amount float64) error
	Credit(ctx context.Context, userID uint, currency string, amount float64) error
	Lock(ctx context.Context, userID uint, currency string, amount float64) error
	Unlock(ctx context.Context, userID uint, currency string, amount float64) error
	GetBalance(ctx context.Context, userID uint, currency string) (balance, locked float64, err error)
}

// PriceSource reads the live mark price and spot index price for a pair.
type PriceSource interface {
	MarkPrice(ctx context.Context, pair string) (float64, bool)
	IndexPrice(ctx context.Context, pair string) (float64, bool)
}

// RateCache stores the latest funding rate for fast UI lookup.
type RateCache interface {
	StoreLatestRate(ctx context.Context, pair string, rate float64)
}

// Broadcaster pushes real-time payloads to a WebSocket channel.
type Broadcaster interface {
	Broadcast(channel string, data interface{})
}

// EventPublisher publishes integration events.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}

// TxManager runs fn inside a DB transaction (carried on ctx).
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

const usdt = "USDT"
