package usecase

import (
	"context"

	"github.com/cryptox/shared/eventbus"
)

// BalanceLocker centralises Redis lock/unlock so every reservation emits a
// balance.changed event for the wallet projector to mirror locked_balance.
type BalanceLocker struct {
	cache BalanceCache
	bus   eventbus.EventPublisher
}

func NewBalanceLocker(cache BalanceCache, bus eventbus.EventPublisher) *BalanceLocker {
	return &BalanceLocker{cache: cache, bus: bus}
}

func (b *BalanceLocker) Lock(ctx context.Context, userID uint, currency string, amount float64) error {
	if err := b.cache.Lock(ctx, userID, currency, amount); err != nil {
		return err
	}
	b.publish(ctx, userID, currency, amount, "lock")
	return nil
}

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
