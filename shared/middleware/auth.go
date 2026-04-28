package middleware

import (
	"strings"

	"github.com/cryptox/shared/utils"
	"github.com/gin-gonic/gin"
)

// JWTAuth reads token from: 1) HttpOnly cookie "access_token" 2) Authorization header
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.JSON(401, gin.H{"success": false, "message": "Token required"})
			c.Abort()
			return
		}
		claims, err := utils.ValidateToken(token, secret)
		if err != nil {
			c.JSON(401, gin.H{"success": false, "message": "Invalid token"})
			c.Abort()
			return
		}
		c.Set("userId", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// extractToken tries cookie first, then Authorization header
func extractToken(c *gin.Context) string {
	// 1. HttpOnly cookie
	if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
		return cookie
	}
	// 2. Authorization: Bearer <token>
	auth := c.GetHeader("Authorization")
	if auth != "" && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "ADMIN" {
			c.JSON(403, gin.H{"success": false, "message": "Admin only"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func GetUserID(c *gin.Context) uint {
	id, exists := c.Get("userId")
	if !exists {
		return 0
	}
	uid, ok := id.(uint)
	if !ok {
		return 0
	}
	return uid
}
