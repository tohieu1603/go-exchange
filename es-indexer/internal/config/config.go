// Package config loads es-indexer configuration from the environment via the
// shared cfg helper.
package config

import (
	"context"

	"github.com/cryptox/shared/cfg"
)

type Config struct {
	cfg.Base
	cfg.Redis
	cfg.Kafka

	ElasticURL string `env:"ELASTIC_URL,default=http://localhost:9200"`
	// HealthPort exposes liveness/readiness probes for this consumer-only worker
	// so an orchestrator can restart it if it wedges or pull it from rotation
	// while Redis is unreachable.
	HealthPort string `env:"HEALTH_PORT,default=8090"`
}

func Load(ctx context.Context) (*Config, error) { return cfg.Load[Config](ctx) }
