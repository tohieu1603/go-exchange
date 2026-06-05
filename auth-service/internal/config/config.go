// Package config loads auth-service configuration from the environment via the
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
	cfg.Kafka

	HTTPPort           string `env:"HTTP_PORT,default=8081"`
	GRPCPort           string `env:"GRPC_PORT,default=9081"`
	JWTSecret          string `env:"JWT_SECRET"`
	WalletGRPCAddr     string `env:"WALLET_GRPC_ADDR,default=localhost:9082"`
	ESURL              string `env:"ES_URL,default=http://localhost:9201"`
	MigrationsDir      string `env:"MIGRATIONS_DIR,default=db/migrations"`
	AuditRetentionDays int    `env:"AUDIT_RETENTION_DAYS,default=90"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
