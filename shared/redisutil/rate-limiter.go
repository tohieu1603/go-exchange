package redisutil

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter provides Redis-backed rate limiting using Lua sliding window.
type RateLimiter struct {
	rdb *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// Allow checks if request is within rate limit.
// key: identifier (e.g., "login:192.168.1.1"), window: duration, maxReq: max requests
func (rl *RateLimiter) Allow(ctx context.Context, key string, window time.Duration, maxReq int) bool {
	result, err := RateLimit.Run(ctx, rl.rdb,
		[]string{fmt.Sprintf("rl:%s", key)},
		int(window.Seconds()),
		maxReq,
		time.Now().UnixMilli(),
	).Int64()
	if err != nil {
		return false // fail-closed: deny on Redis errors (security over availability)
	}
	return result == 1
}
