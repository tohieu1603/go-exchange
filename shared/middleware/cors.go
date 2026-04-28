package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins returns configured origins or defaults for dev.
func allowedOrigins() []string {
	env := os.Getenv("CORS_ORIGINS")
	if env != "" {
		return strings.Split(env, ",")
	}
	return []string{"http://localhost:3000", "http://localhost:3001"}
}

func CORS() gin.HandlerFunc {
	origins := allowedOrigins()
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
