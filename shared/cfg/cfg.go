// Package cfg provides typed environment configuration via sethvargo/go-envconfig
// (mirrors the appios platform/config convention). Services embed the shared
// structs into their own config type and call Load; a missing required var makes
// the service refuse to start.
package cfg

import (
	"context"
	"fmt"

	"github.com/sethvargo/go-envconfig"
)

// Base holds fields common to every service.
type Base struct {
	Env      string `env:"ENV,default=development"`
	LogLevel string `env:"LOG_LEVEL,default=info"`
}

// Postgres holds PostgreSQL connection settings. A full DB_DSN takes precedence;
// otherwise the DSN is assembled from the component vars (DB_HOST/PORT/USER/...)
// the existing deployment already sets, so the appios stack drops in without a
// compose change.
type Postgres struct {
	DSN      string `env:"DB_DSN"`
	Host     string `env:"DB_HOST,default=localhost"`
	Port     string `env:"DB_PORT,default=5432"`
	User     string `env:"DB_USER,default=postgres"`
	Password string `env:"DB_PASSWORD,default=postgres"`
	Name     string `env:"DB_NAME,default=exchange"`
	SSLMode  string `env:"DB_SSL_MODE,default=disable"`
	MaxConns int32  `env:"DB_MAX_CONNS,default=10"`
}

// Dsn returns the explicit DB_DSN when set, else a postgres:// URL built from the
// component vars.
func (p Postgres) Dsn() string {
	if p.DSN != "" {
		return p.DSN
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.Name, p.SSLMode)
}

// Redis holds Redis connection settings (URL form, e.g. redis://host:6379).
type Redis struct {
	URL string `env:"REDIS_URL,default=redis://localhost:6379"`
}

// Kafka holds Kafka broker settings (empty => async falls back to Redis
// Streams). go-envconfig comma-splits the env value into the slice.
type Kafka struct {
	Brokers []string `env:"KAFKA_BROKERS"`
}

// GRPC holds the gRPC listen address.
type GRPC struct {
	Addr string `env:"GRPC_ADDR"`
}

// HTTP holds the REST listen port.
type HTTP struct {
	Port string `env:"HTTP_PORT,default=8080"`
}

// Load populates a config struct T from the process environment.
func Load[T any](ctx context.Context) (*T, error) {
	var c T
	if err := envconfig.Process(ctx, &c); err != nil {
		return nil, fmt.Errorf("cfg: load: %w", err)
	}
	return &c, nil
}
