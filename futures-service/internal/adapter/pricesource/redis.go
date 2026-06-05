// Package pricesource adapts Redis price/index reads onto
// application.PriceSource and application.RateCache.
package pricesource

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis reads mark/index prices (written by market-service) and caches the
// latest funding rate.
type Redis struct{ rdb *redis.Client }

func NewRedis(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

func (r *Redis) MarkPrice(ctx context.Context, pair string) (float64, bool) {
	v, err := r.rdb.Get(ctx, "price:"+pair).Float64()
	if err != nil {
		return 0, false
	}
	return v, true
}

func (r *Redis) IndexPrice(ctx context.Context, pair string) (float64, bool) {
	v, err := r.rdb.Get(ctx, "index:"+pair).Float64()
	if err != nil {
		return 0, false
	}
	return v, true
}

func (r *Redis) StoreLatestRate(ctx context.Context, pair string, rate float64) {
	r.rdb.Set(ctx, "funding:"+pair, rate, 9*time.Hour)
}
