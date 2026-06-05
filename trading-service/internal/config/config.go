// Package config loads trading-service configuration from the environment via
// the shared cfg helper.
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

type Config struct {
	cfg.Base
	cfg.Postgres
	cfg.Redis
	cfg.Kafka

	HTTPPort       string `env:"HTTP_PORT,default=8084"`
	JWTSecret      string `env:"JWT_SECRET"`
	WalletGRPCAddr string `env:"WALLET_GRPC_ADDR,default=localhost:9082"`
	MarketGRPCAddr string `env:"MARKET_GRPC_ADDR,default=localhost:9083"`
	MigrationsDir  string `env:"MIGRATIONS_DIR,default=db/migrations"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
