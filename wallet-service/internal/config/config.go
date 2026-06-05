// Package config loads wallet-service configuration from the environment via the
// shared cfg helper (appios platform/config convention).
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

// Config is the full wallet-service configuration. The embedded shared structs
// contribute DB/Redis/Kafka/base settings; the fields below are service-specific.
type Config struct {
	cfg.Base
	cfg.Postgres
	cfg.Redis
	cfg.Kafka

	HTTPPort      string `env:"HTTP_PORT,default=8082"`
	GRPCPort      string `env:"GRPC_PORT,default=9082"`
	JWTSecret     string `env:"JWT_SECRET"`
	AuthGRPCAddr  string `env:"AUTH_GRPC_ADDR,default=localhost:9081"`
	MigrationsDir string `env:"MIGRATIONS_DIR,default=db/migrations"`

	SepayBankCode    string `env:"SEPAY_BANK_CODE"`
	SepayBankAccount string `env:"SEPAY_BANK_ACCOUNT"`
	SepayAccountName string `env:"SEPAY_ACCOUNT_NAME"`
	SepayAPIKey      string `env:"SEPAY_API_KEY"`
	SepaySecretKey   string `env:"SEPAY_SECRET_KEY"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
