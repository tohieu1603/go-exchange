package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cryptox/es-indexer/internal/indexer"
	"github.com/cryptox/shared/config"
	"github.com/cryptox/shared/eventbus"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.LoadBase()
	esURL := getEnv("ELASTIC_URL", "http://localhost:9200")

	// Redis (for eventbus fallback)
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis parse: %v", err)
	}
	rdb := redis.NewClient(opt)

	// Elasticsearch client
	es, err := indexer.NewESClient([]string{esURL})
	if err != nil {
		log.Fatalf("elasticsearch connect: %v", err)
	}

	// Create indexes with mappings
	for name, mapping := range indexer.IndexMappings {
		es.EnsureIndex(name, mapping)
	}

	// Event bus (Kafka or Redis Streams)
	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	// Handlers
	handlers := indexer.NewHandlers(es)

	// Subscribe to all topics — separate consumer groups for fan-out
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Subscribe(eventbus.TopicTradeExecuted, handlers.HandleTrade)
	bus.StartConsumer(ctx, eventbus.TopicTradeExecuted, "es-trade-indexer", "worker-1")

	bus.Subscribe(eventbus.TopicOrderUpdated, handlers.HandleOrder)
	bus.StartConsumer(ctx, eventbus.TopicOrderUpdated, "es-order-indexer", "worker-1")

	bus.Subscribe(eventbus.TopicBalanceChanged, handlers.HandleBalance)
	bus.StartConsumer(ctx, eventbus.TopicBalanceChanged, "es-balance-indexer", "worker-1")

	bus.Subscribe(eventbus.TopicNotificationCreated, handlers.HandleNotification)
	bus.StartConsumer(ctx, eventbus.TopicNotificationCreated, "es-notification-indexer", "worker-1")

	// Audit log → "audit_logs" ES index → Kibana security dashboards.
	bus.Subscribe(eventbus.TopicAuditLogged, handlers.HandleAudit)
	bus.StartConsumer(ctx, eventbus.TopicAuditLogged, "es-audit-indexer", "worker-1")

	log.Printf("[es-indexer] started — 5 consumers (trade, order, balance, notification, audit) → %s", esURL)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[es-indexer] shutdown")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
