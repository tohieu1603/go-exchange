// Package config loads market-service configuration from the environment via the
// shared cfg helper.
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

type Config struct {
	cfg.Base
	cfg.Postgres
	cfg.Redis

	HTTPPort      string `env:"HTTP_PORT,default=8083"`
	GRPCPort      string `env:"GRPC_PORT,default=9083"`
	JWTSecret     string `env:"JWT_SECRET"`
	MigrationsDir string `env:"MIGRATIONS_DIR,default=db/migrations"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
