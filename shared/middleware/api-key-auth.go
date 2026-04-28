package middleware

import (
	"bytes"
	"io"

	"github.com/gin-gonic/gin"
)

// APIKeyValidator is implemented by auth-service (or its gRPC client).
// Returning (userID, role, permissions CSV, error).
type APIKeyValidator interface {
	Validate(keyID, signature, timestamp, method, path string, body []byte, clientIP string) (userID uint, role, permissions string, err error)
}

// APIKeyAuth authenticates HMAC-signed requests from algorithmic traders.
// On success, sets c.Set("userId" ...) like JWTAuth does, plus apiPermissions.
//
// Headers checked:
//   X-API-Key
//   X-API-Sign
//   X-API-Timestamp
//
// If the X-API-Key header is absent, this middleware is a no-op (next handler
// can fall back to JWTAuth). Mount BEFORE JWTAuth in the chain.
func APIKeyAuth(v APIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID := c.GetHeader("X-API-Key")
		if keyID == "" {
			c.Next()
			return
		}
		sig := c.GetHeader("X-API-Sign")
		ts := c.GetHeader("X-API-Timestamp")
		if sig == "" || ts == "" {
			c.JSON(401, gin.H{"success": false, "message": "missing API signature"})
			c.Abort()
			return
		}

		// Read body once and rewind for the next handler.
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		userID, role, perms, err := v.Validate(
			keyID, sig, ts,
			c.Request.Method, c.Request.URL.Path,
			bodyBytes, c.ClientIP(),
		)
		if err != nil {
			c.JSON(401, gin.H{"success": false, "message": err.Error()})
			c.Abort()
			return
		}
		c.Set("userId", userID)
		c.Set("role", role)
		c.Set("apiPermissions", perms)
		c.Set("authMethod", "api_key")
		c.Next()
	}
}

// RequireAPIPermission gates a route to API-key callers holding `perm`
// (e.g. "trade", "withdraw"). Cookie/JWT-auth callers pass through.
//
// Detects API-key auth via the X-API-Permissions header set by the gateway
// after successful HMAC validation. Empty header = cookie/JWT auth = pass.
func RequireAPIPermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Header set by gateway proxy after API-key auth
		if perms := c.GetHeader("X-API-Permissions"); perms != "" {
			if !csvContains(perms, perm) {
				c.JSON(403, gin.H{
					"success": false,
					"message": "API key lacks permission: " + perm,
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}
		// 2. Same-process middleware-set context (when service mounts APIKeyAuth directly)
		if permsRaw, exists := c.Get("apiPermissions"); exists {
			perms, _ := permsRaw.(string)
			if !csvContains(perms, perm) {
				c.JSON(403, gin.H{
					"success": false,
					"message": "API key lacks permission: " + perm,
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func csvContains(csv, want string) bool {
	for _, p := range splitCSV(csv) {
		if p == want {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, c := range s {
		if c == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		if c == ' ' {
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
