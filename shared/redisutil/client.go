package redisutil

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// pingTimeout bounds the startup connectivity check.
const pingTimeout = 5 * time.Second

// NewClient parses a redis:// URL, builds a client, and verifies connectivity
// with a bounded ping. Every service previously inlined
//
//	opt, _ := redis.ParseURL(cfg.RedisURL)
//	rdb := redis.NewClient(opt)
//
// — sometimes swallowing the parse error and sometimes pinging, sometimes not.
// NewClient makes that consistent and surfaces the error for callers that want
// to decide; MustClient is the fail-fast variant for cmd/main.go.
func NewClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return rdb, nil
}

// MustClient is NewClient but terminates the process on failure.
func MustClient(redisURL string) *redis.Client {
	rdb, err := NewClient(redisURL)
	if err != nil {
		panic(fmt.Sprintf("redisutil.MustClient: %v", err))
	}
	return rdb
}
