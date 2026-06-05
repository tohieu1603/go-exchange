// Package feewallet resolves the platform fee-collection wallet user ID from
// Redis at startup (written by auth-service) and hands it to the application.
package feewallet

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/cryptox/shared/types"
	"github.com/redis/go-redis/v9"
)

// Resolve polls Redis (up to 30s with backoff) for the fee wallet user ID and
// passes it to set. Auth-service sets the key on its first boot.
func Resolve(rdb *redis.Client, set func(uint)) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for attempt := 0; attempt < 30; attempt++ {
		if val, err := rdb.Get(ctx, types.RedisKeyFeeWalletID).Result(); err == nil && val != "" {
			if id, perr := strconv.ParseUint(val, 10, 64); perr == nil && id > 0 {
				set(uint(id))
				log.Printf("[fees] platform fee wallet user_id=%d", id)
				return
			}
		}
		select {
		case <-ctx.Done():
			log.Printf("[fees] fee wallet ID not set after timeout — fees will not be credited")
			return
		case <-time.After(time.Second):
		}
	}
}
