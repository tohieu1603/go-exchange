package redisutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// newRedis returns a real-ish redis-client backed by an in-process miniredis.
// miniredis supports the Lua scripting we use for the sliding-window window.
func newRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rdb, _ := newRedis(t)
	rl := NewRateLimiter(rdb)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow(ctx, "user:1", time.Minute, 5),
			"request %d should be allowed within 5/min", i+1)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rdb, _ := newRedis(t)
	rl := NewRateLimiter(rdb)
	ctx := context.Background()

	// Fill the bucket.
	for i := 0; i < 3; i++ {
		require.True(t, rl.Allow(ctx, "user:1", time.Minute, 3))
	}
	// 4th must be blocked.
	assert.False(t, rl.Allow(ctx, "user:1", time.Minute, 3),
		"4th request should be rate-limited")
}

func TestRateLimiter_KeysIsolated(t *testing.T) {
	// Two different keys must have independent buckets — otherwise one
	// noisy IP would starve everyone else.
	rdb, _ := newRedis(t)
	rl := NewRateLimiter(rdb)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.True(t, rl.Allow(ctx, "key-a", time.Minute, 3))
	}
	require.False(t, rl.Allow(ctx, "key-a", time.Minute, 3))
	// Different key, fresh bucket.
	assert.True(t, rl.Allow(ctx, "key-b", time.Minute, 3))
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	// After fast-forwarding past the window, the bucket should reset.
	rdb, mr := newRedis(t)
	rl := NewRateLimiter(rdb)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		require.True(t, rl.Allow(ctx, "user:fwd", 5*time.Second, 2))
	}
	require.False(t, rl.Allow(ctx, "user:fwd", 5*time.Second, 2))

	// Move time forward past the window so old entries are evicted.
	mr.FastForward(6 * time.Second)
	assert.True(t, rl.Allow(ctx, "user:fwd", 5*time.Second, 2),
		"after window expiry the bucket should reset")
}

func TestRateLimiter_FailClosedOnRedisError(t *testing.T) {
	// Closed client → Redis error → must DENY (security over availability).
	rdb, mr := newRedis(t)
	mr.Close()
	rl := NewRateLimiter(rdb)
	assert.False(t, rl.Allow(context.Background(), "x", time.Minute, 100),
		"Allow must fail-closed when Redis is unreachable")
}

func TestRateLimiter_GinMiddleware_429OnExcess(t *testing.T) {
	rdb, _ := newRedis(t)
	rl := NewRateLimiter(rdb)
	r := gin.New()
	r.Use(rl.GinMiddleware("test", time.Minute, 2))
	r.GET("/p", func(c *gin.Context) { c.String(200, "ok") })

	hit := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/p", nil)
		req.RemoteAddr = "203.0.113.1:1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, 200, hit())
	assert.Equal(t, 200, hit())
	assert.Equal(t, http.StatusTooManyRequests, hit(), "3rd should 429")
}
