package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisFeeResolver reads (maker, taker) rates from Redis keys populated by
// the auth-service fee-tier worker.
//
// Keys:
//   fee_tier:{userID}:maker  → "0.0009"
//   fee_tier:{userID}:taker  → "0.0010"
//
// On cache miss, returns the configured default (typically VIP0).
type RedisFeeResolver struct {
	rdb          *redis.Client
	defaultMaker float64
	defaultTaker float64
}

func NewRedisFeeResolver(rdb *redis.Client, defaultMaker, defaultTaker float64) *RedisFeeResolver {
	return &RedisFeeResolver{rdb: rdb, defaultMaker: defaultMaker, defaultTaker: defaultTaker}
}

func (r *RedisFeeResolver) Rates(userID uint) (float64, float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	mk := fmt.Sprintf("fee_tier:%d:maker", userID)
	tk := fmt.Sprintf("fee_tier:%d:taker", userID)
	maker, err1 := r.rdb.Get(ctx, mk).Float64()
	taker, err2 := r.rdb.Get(ctx, tk).Float64()
	if err1 != nil || err2 != nil {
		return r.defaultMaker, r.defaultTaker
	}
	return maker, taker
}
