// Package cache is the Redis adapter for usecase.CandleCache.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cryptox/market-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

const ttl = 30 * time.Second

// CandleCache caches rendered candle series in Redis for a short TTL.
type CandleCache struct{ rdb *redis.Client }

func NewCandleCache(rdb *redis.Client) *CandleCache { return &CandleCache{rdb: rdb} }

func key(pair, interval string, limit int) string {
	return fmt.Sprintf("candles:%s:%s:%d", pair, interval, limit)
}

func (c *CandleCache) Get(ctx context.Context, pair, interval string, limit int) ([]domain.CandleWS, bool) {
	raw, err := c.rdb.Get(ctx, key(pair, interval, limit)).Bytes()
	if err != nil {
		return nil, false
	}
	var out []domain.CandleWS
	if json.Unmarshal(raw, &out) != nil {
		return nil, false
	}
	return out, true
}

func (c *CandleCache) Set(ctx context.Context, pair, interval string, limit int, candles []domain.CandleWS) {
	if b, err := json.Marshal(candles); err == nil {
		c.rdb.Set(ctx, key(pair, interval, limit), b, ttl)
	}
}

func (c *CandleCache) Invalidate(ctx context.Context, pair, interval string) {
	pattern := fmt.Sprintf("candles:%s:%s:*", pair, interval)
	if keys, _ := c.rdb.Keys(ctx, pattern).Result(); len(keys) > 0 {
		c.rdb.Del(ctx, keys...)
	}
}
