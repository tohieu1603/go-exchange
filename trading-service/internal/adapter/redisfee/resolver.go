// Package redisfee adapts the auth-service fee-tier Redis keys onto
// application.FeeResolver.
package redisfee

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Resolver reads (maker, taker) rates from fee_tier:{userID}:{maker|taker},
// falling back to the configured defaults on a cache miss.
type Resolver struct {
	rdb          *redis.Client
	defaultMaker float64
	defaultTaker float64
}

func NewResolver(rdb *redis.Client, defaultMaker, defaultTaker float64) *Resolver {
	return &Resolver{rdb: rdb, defaultMaker: defaultMaker, defaultTaker: defaultTaker}
}

func (r *Resolver) Rates(userID uint) (float64, float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	maker, err1 := r.rdb.Get(ctx, fmt.Sprintf("fee_tier:%d:maker", userID)).Float64()
	taker, err2 := r.rdb.Get(ctx, fmt.Sprintf("fee_tier:%d:taker", userID)).Float64()
	if err1 != nil || err2 != nil {
		return r.defaultMaker, r.defaultTaker
	}
	return maker, taker
}
