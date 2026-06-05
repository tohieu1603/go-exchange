// Package config loads gateway configuration from the environment via the shared
// cfg helper. The gateway is the edge reverse proxy: it holds the upstream
// service URLs and the auth gRPC address it validates tokens/API-keys against.
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

type Config struct {
	cfg.Base
	cfg.Redis

	Port      string `env:"PORT,default=8080"`
	JWTSecret string `env:"JWT_SECRET"`

	AuthURL         string `env:"AUTH_URL,default=http://localhost:8081"`
	WalletURL       string `env:"WALLET_URL,default=http://localhost:8082"`
	MarketURL       string `env:"MARKET_URL,default=http://localhost:8083"`
	TradingURL      string `env:"TRADING_URL,default=http://localhost:8084"`
	FuturesURL      string `env:"FUTURES_URL,default=http://localhost:8085"`
	NotificationURL string `env:"NOTIFICATION_URL,default=http://localhost:8086"`
	AuthGRPCAddr    string `env:"AUTH_GRPC_ADDR,default=localhost:9081"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
