// Command server is the es-indexer composition root: it wires the Elasticsearch
// adapter, ensures the index mappings, and starts one Kafka/Redis-Streams
// consumer per projected event stream (trade, order, balance, notification,
// audit). It is a consumer-only worker — no HTTP/gRPC surface.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	elastic "github.com/cryptox/es-indexer/internal/adapter/elasticsearch"
	"github.com/cryptox/es-indexer/internal/config"
	"github.com/cryptox/es-indexer/internal/usecase"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/redisutil"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rdb := redisutil.MustClient(cfg.URL)

	// Elasticsearch adapter (implements usecase.Indexer).
	es, err := elastic.New([]string{cfg.ElasticURL})
	if err != nil {
		log.Fatalf("elasticsearch connect: %v", err)
	}
	for name, mapping := range elastic.IndexMappings {
		es.EnsureIndex(name, mapping)
	}

	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)
	handlers := usecase.NewHandlers(es)

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

	// Liveness/readiness probe server (stdlib only — this worker stays gin-free).
	healthSrv := newHealthServer(cfg.HealthPort, func(c context.Context) error { return rdb.Ping(c).Err() })
	go func() {
		if err := httpx.ListenAndServe(healthSrv); err != nil {
			log.Printf("[es-indexer] health server: %v", err)
		}
	}()

	log.Printf("[es-indexer] started — 5 consumers (trade, order, balance, notification, audit) → %s", cfg.ElasticURL)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[es-indexer] shutting down...")
	_ = httpx.Shutdown(healthSrv, 5*time.Second)
	log.Println("[es-indexer] shutdown")
}

// newHealthServer builds the worker's probe server: /healthz is unconditional
// liveness; /readyz runs ready (e.g. a Redis ping) and reports 503 if it fails.
// Errors are reported as a status only and never written to the response body.
func newHealthServer(port string, ready func(context.Context) error) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"status":"ok","service":"es-indexer"}`)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			log.Printf("[es-indexer] readiness check failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, `{"status":"degraded","service":"es-indexer"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"ok","service":"es-indexer"}`)
	})
	return httpx.NewServer(":"+port, mux)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
