package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cryptox/shared/redisutil"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func TestRateLimit_429OnExcess(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	rl := redisutil.NewRateLimiter(rdb)
	r := gin.New()
	r.Use(RateLimit(rl, "test", time.Minute, 2))
	r.GET("/p", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	hit := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/p", nil)
		req.RemoteAddr = "203.0.113.1:1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, hit())
	assert.Equal(t, http.StatusOK, hit())
	assert.Equal(t, http.StatusTooManyRequests, hit(), "3rd request should be rate-limited")
}

// Distinct client IPs must not share a bucket.
func TestRateLimit_PerIPIsolation(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	rl := redisutil.NewRateLimiter(rdb)
	r := gin.New()
	r.Use(RateLimit(rl, "iso", time.Minute, 1))
	r.GET("/p", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	hit := func(ip string) int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/p", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, hit("203.0.113.1"))
	assert.Equal(t, http.StatusTooManyRequests, hit("203.0.113.1"), "same IP over limit")
	assert.Equal(t, http.StatusOK, hit("203.0.113.2"), "different IP has its own bucket")
}
