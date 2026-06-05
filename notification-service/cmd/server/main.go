// @title           Notification Service API
// @version         1.0
// @description     User notifications: list, mark read, unread count
// @host            localhost:8086
// @BasePath        /api

// Command server is the notification-service composition root: it loads config,
// opens the pgx pool + runs migrations, wires the usecase over its adapters, and
// starts the Kafka/Redis-Streams consumer + the REST server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cryptox/notification-service/internal/adapter/postgres"
	"github.com/cryptox/notification-service/internal/config"
	httptransport "github.com/cryptox/notification-service/internal/transport/http"
	"github.com/cryptox/notification-service/internal/usecase"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/notification-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("notification")
	defer tracingShutdown(context.Background())

	// Infrastructure: pgx pool + migrations (sqlc-generated queries run on it).
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "notification_schema_migrations"); err != nil {
		log.Printf("[notification] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()
	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)

	// Application
	uc := usecase.NewNotificationUseCase(postgres.NewRepo(pool), hub)

	// Inbound: notification.created consumer (async via Kafka or Redis Streams).
	bus.Subscribe(eventbus.TopicNotificationCreated, func(c context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.NotificationEvent](data)
		if err != nil {
			log.Printf("[notification] unmarshal error: %v", err)
			return nil
		}
		return uc.Notify(c, ev.UserID, ev.Type, ev.Title, ev.Message, ev.Pair)
	})
	bus.StartConsumer(ctx, eventbus.TopicNotificationCreated, "notification-service", "worker-1")

	// Inbound: REST
	h := httptransport.NewHandler(uc)
	r := gin.New()
	r.Use(gin.Logger(), middleware.Recovery(), middleware.CORS(), otelgin.Middleware("notification"), metrics.GinMiddleware("notification"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	health.New("notification").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	auth := r.Group("/api/notifications", middleware.JWTAuth(cfg.JWTSecret))
	auth.GET("", h.List)
	auth.GET("/unread-count", h.UnreadCount)
	auth.POST("/read-all", h.MarkAllRead)
	auth.POST("/:id/read", h.MarkRead)

	srv := httpx.NewServer(":"+cfg.HTTPPort, r)
	go func() {
		log.Printf("notification-service HTTP listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	log.Println("notification-service shutdown")
}
