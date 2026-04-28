package service

import (
	"context"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// FeeResolver returns maker/taker fee rates for a user.
// Implementations may consult fee-tier tables based on 30-day volume.
type FeeResolver interface {
	// Rates returns (maker, taker) fee rates as decimals (0.001 = 0.1%).
	Rates(userID uint) (maker, taker float64)
}

// FlatFeeResolver — fixed rates regardless of user. Default fallback.
type FlatFeeResolver struct {
	Maker float64
	Taker float64
}

func NewFlatFeeResolver(maker, taker float64) *FlatFeeResolver {
	return &FlatFeeResolver{Maker: maker, Taker: taker}
}

func (r *FlatFeeResolver) Rates(_ uint) (float64, float64) {
	return r.Maker, r.Taker
}

// RedisKeyFeeWalletID is the canonical Redis key holding the user ID of the
// platform fee-collection account. Auth-service writes it on startup;
// trading/futures services read it with retry.
const RedisKeyFeeWalletID = "system:fee_wallet_id"

// platformFeeUserID is loaded once at startup and read on every trade.
// Stored atomically so we can update it without a lock.
var platformFeeUserID atomic.Uint64

// PlatformFeeUserID returns the resolved fee wallet user ID.
// Returns 0 if not yet resolved — callers should check and skip fee crediting.
func PlatformFeeUserID() uint {
	return uint(platformFeeUserID.Load())
}

// ResolveFeeWalletID polls Redis for the fee wallet user ID at startup.
// Blocks (with retry/backoff) up to 30 seconds, then returns whatever it has.
// Auth-service is responsible for setting this key on its first boot.
func ResolveFeeWalletID(rdb *redis.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for attempt := 0; attempt < 30; attempt++ {
		val, err := rdb.Get(ctx, RedisKeyFeeWalletID).Result()
		if err == nil && val != "" {
			if id, perr := strconv.ParseUint(val, 10, 64); perr == nil && id > 0 {
				platformFeeUserID.Store(id)
				log.Printf("[fees] platform fee wallet user_id=%d", id)
				return
			}
		}
		select {
		case <-ctx.Done():
			log.Printf("[fees] fee wallet ID not set after timeout — fees will not be credited")
			return
		case <-time.After(time.Second):
		}
	}
}
