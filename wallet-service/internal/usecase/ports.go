// Package usecase holds the wallet application business rules. It orchestrates
// the domain aggregates through the domain repository ports and the outbound
// ports declared here. It imports domain + shared libraries only — never pgx,
// gin, or gRPC stubs (those live in adapter/transport).
package usecase

import (
	"context"

	"github.com/cryptox/shared/types"
)

// TxManager runs fn inside a single database transaction. The active pgx.Tx is
// carried on the context so repository adapters pick it up transparently — the
// transaction primitive never leaks into the domain or application signatures.
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// EventPublisher publishes integration events to the message bus
// (shared/eventbus satisfies this directly).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}

// BalanceCache is the Redis hot-path balance store (atomic Lua ops).
// *redisutil.BalanceCache satisfies this directly.
type BalanceCache interface {
	GetBalance(ctx context.Context, userID uint, currency string) (float64, bool)
	GetLocked(ctx context.Context, userID uint, currency string) float64
	LoadFromDB(ctx context.Context, userID uint, currency string, balance, locked float64)
	// WarmIfMissing seeds the cache from DB only when no value is present, so a
	// service restart cannot clobber live balances if Redis stayed up.
	WarmIfMissing(ctx context.Context, userID uint, currency string, balance, locked float64)
	Deduct(ctx context.Context, userID uint, currency string, amount float64) (float64, error)
	Credit(ctx context.Context, userID uint, currency string, amount float64) (float64, error)
	Lock(ctx context.Context, userID uint, currency string, amount float64) error
	Unlock(ctx context.Context, userID uint, currency string, amount float64) error
}

// SettingsGate exposes operational policy + per-user gates sourced from Redis
// (platform settings written by auth-service, account-lock and bonus flags).
type SettingsGate interface {
	PlatformSettings(ctx context.Context) types.PlatformSettings
	AccountLocked(ctx context.Context, userID uint) bool
	BonusRemaining(ctx context.Context, userID uint) float64
}

// RateProvider supplies the current VND-per-USDT exchange rate.
type RateProvider interface {
	VNDRate() float64
}

// TwoFAVerifier verifies a user's TOTP code via auth-service. required=false
// means 2FA is not enabled for the user.
type TwoFAVerifier interface {
	Verify2FA(ctx context.Context, userID uint, code string) (valid, required bool, err error)
}
