// Package settings adapts Redis-sourced operational policy onto the
// application.SettingsGate port.
package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cryptox/shared/types"
	"github.com/redis/go-redis/v9"
)

const platformSettingsKey = "platform_settings"

// RedisGate reads platform settings + per-user flags written by other services.
type RedisGate struct{ rdb *redis.Client }

func NewRedisGate(rdb *redis.Client) *RedisGate { return &RedisGate{rdb: rdb} }

// PlatformSettings reads the JSON blob written by auth-service, falling back to
// safe defaults when absent or malformed.
func (g *RedisGate) PlatformSettings(ctx context.Context) types.PlatformSettings {
	if raw, err := g.rdb.Get(ctx, platformSettingsKey).Bytes(); err == nil {
		var ps types.PlatformSettings
		if json.Unmarshal(raw, &ps) == nil {
			return ps
		}
	}
	return types.DefaultPlatformSettings()
}

func (g *RedisGate) AccountLocked(ctx context.Context, userID uint) bool {
	v, _ := g.rdb.Get(ctx, fmt.Sprintf("user_locked:%d", userID)).Result()
	return v == "true"
}

func (g *RedisGate) BonusRemaining(ctx context.Context, userID uint) float64 {
	v, _ := g.rdb.Get(ctx, fmt.Sprintf("user_bonus_remaining:%d", userID)).Float64()
	return v
}
