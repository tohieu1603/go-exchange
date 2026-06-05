// Package config loads notification-service configuration from the environment
// (appios platform/config convention: typed, validated on boot).
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

// Config is the notification-service configuration.
type Config struct {
	cfg.Base
	cfg.Postgres
	cfg.Redis
	cfg.Kafka
	HTTPPort      string `env:"HTTP_PORT,default=8086"`
	JWTSecret     string `env:"JWT_SECRET"`
	MigrationsDir string `env:"MIGRATIONS_DIR,default=db/migrations"`
}

// Load reads the configuration from the process environment.
func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
