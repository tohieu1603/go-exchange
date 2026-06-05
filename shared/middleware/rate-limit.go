package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cryptox/shared/redisutil"
	"github.com/gin-gonic/gin"
)

// RateLimit returns a Gin middleware that rate-limits requests by client IP
// using the supplied Redis-backed limiter.
//
//	prefix  namespaces the key (e.g. "login", "refresh")
//	window  the sliding window duration
//	maxReq  the maximum requests allowed per window per IP
//
// This lives in the middleware package — not redisutil — so that redisutil
// stays a pure infrastructure package with no web-framework dependency. Pure
// consumers such as es-indexer can then use redisutil.MustClient / BalanceCache
// without transitively pulling in gin.
func RateLimit(rl *redisutil.RateLimiter, prefix string, window time.Duration, maxReq int) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("%s:%s", prefix, c.ClientIP())
		if !rl.Allow(c.Request.Context(), key, window, maxReq) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "rate limit exceeded, try again later",
			})
			return
		}
		c.Next()
	}
}
