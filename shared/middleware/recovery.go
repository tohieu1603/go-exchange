package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// Recovery returns a Gin middleware that recovers from a panic in any handler,
// logs the panic with its stack trace server-side, and responds with a generic
// 500 "internal error" — never the panic value or stack, which could expose
// internal state. It replaces gin.Recovery() so the error envelope matches the
// rest of the API (response.Response shape) instead of gin's default body.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic] %s %s: %v\n%s", c.Request.Method, c.Request.URL.Path, r, debug.Stack())
				if !c.Writer.Written() {
					response.Error(c, http.StatusInternalServerError, "internal error")
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
