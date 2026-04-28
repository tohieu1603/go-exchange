package middleware

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// KYCGate blocks trading for users who are locked or have not completed KYC (kycStep < 4).
// Reads state from Redis keys written by auth-service on login / KYC transitions:
//   - user_locked:{userID}  = "true"
//   - kyc:{userID}          = kycStep (int)
func KYCGate(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == 0 {
			c.JSON(401, gin.H{"success": false, "message": "unauthorized"})
			c.Abort()
			return
		}

		ctx := context.Background()

		// Check account lock
		locked, _ := rdb.Get(ctx, fmt.Sprintf("user_locked:%d", userID)).Result()
		if locked == "true" {
			c.JSON(403, gin.H{"success": false, "message": "account is locked"})
			c.Abort()
			return
		}

		// Check KYC step (4 = approved/verified)
		kycStep, err := rdb.Get(ctx, fmt.Sprintf("kyc:%d", userID)).Int()
		if err != nil || kycStep < 4 {
			c.JSON(403, gin.H{"success": false, "message": "KYC verification required to trade"})
			c.Abort()
			return
		}

		c.Next()
	}
}
