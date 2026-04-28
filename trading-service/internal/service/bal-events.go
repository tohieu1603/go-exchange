package service

import (
	"context"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/redisutil"
)

// BalanceLocker is a thin Redis Lua + event-publish wrapper used by the
// trading-service hot path. Centralized so every Lock/Unlock emits a
// balance.changed event for the wallet-service projector to update the
// locked_balance column in PostgreSQL.
type BalanceLocker struct {
	cache *redisutil.BalanceCache
	bus   eventbus.EventPublisher
}

func NewBalanceLocker(cache *redisutil.BalanceCache, bus eventbus.EventPublisher) *BalanceLocker {
	return &BalanceLocker{cache: cache, bus: bus}
}

// Lock atomically increases locked balance and emits balance.changed reason="lock".
func (b *BalanceLocker) Lock(ctx context.Context, userID uint, currency string, amount float64) error {
	if err := b.cache.Lock(ctx, userID, currency, amount); err != nil {
		return err
	}
	b.publish(ctx, userID, currency, amount, "lock")
	return nil
}

// Unlock decreases locked balance and emits balance.changed reason="unlock".
func (b *BalanceLocker) Unlock(ctx context.Context, userID uint, currency string, amount float64) {
	_ = b.cache.Unlock(ctx, userID, currency, amount)
	b.publish(ctx, userID, currency, amount, "unlock")
}

func (b *BalanceLocker) publish(ctx context.Context, userID uint, currency string, amount float64, reason string) {
	if b.bus == nil {
		return
	}
	_ = b.bus.Publish(ctx, eventbus.TopicBalanceChanged, eventbus.BalanceEvent{
		UserID: userID, Currency: currency, Delta: amount, Reason: reason,
	})
}
