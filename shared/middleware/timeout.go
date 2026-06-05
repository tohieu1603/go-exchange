package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout returns a Gin middleware that bounds each request's context to d.
// Downstream work that honours context cancellation (database queries, gRPC
// calls, Redis ops) is aborted when the deadline passes, so a slow dependency
// cannot pin a request — and the goroutine/connection it holds — indefinitely
// under load. It deliberately does not write a response itself (which would
// race the handler); it only propagates the deadline so handlers fail fast and
// return their own error envelope.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
