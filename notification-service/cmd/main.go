// @title           Notification Service API
// @version         1.0
// @description     User notifications: list, mark read, unread count
// @host            localhost:8086
// @BasePath        /api

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cryptox/notification-service/internal/handler"
	"github.com/cryptox/notification-service/internal/repository"
	"github.com/cryptox/notification-service/internal/service"
	"github.com/cryptox/shared/config"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/notification-service/internal/model"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/notification-service/cmd/docs"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("notification")
	defer tracingShutdown(context.Background())
	httpPort := getEnv("HTTP_PORT", "8086")

	// DB
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	db.AutoMigrate(&model.Notification{})

	// Redis
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis parse URL: %v", err)
	}
	rdb := redis.NewClient(opt)

	// WS Hub (broadcasts notifications to connected clients via Redis Pub/Sub)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()

	// Repos + Service
	notifRepo := repository.NewNotificationRepo(db)
	notifSvc := service.NewNotificationService(notifRepo, hub)

	// EventBus: Kafka consumer for notification.created topic
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	bus.Subscribe(eventbus.TopicNotificationCreated, func(_ context.Context, _ string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.NotificationEvent](data)
		if err != nil {
			log.Printf("[notification] unmarshal error: %v", err)
			return nil
		}
		notifSvc.PersistAndBroadcast(event)
		return nil
	})
	bus.StartConsumer(ctx, eventbus.TopicNotificationCreated, "notification-service", "worker-1")

	// Handler + HTTP routes
	notifH := handler.NewNotificationHandler(notifSvc)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(), otelgin.Middleware("notification"), metrics.GinMiddleware("notification"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	auth := r.Group("/api/notifications", middleware.JWTAuth(cfg.JWTSecret))
	auth.GET("", notifH.List)
	auth.GET("/unread-count", notifH.UnreadCount)
	auth.POST("/read-all", notifH.MarkAllRead)
	auth.POST("/:id/read", notifH.MarkRead)

	// HTTP server with graceful shutdown
	srv := &http.Server{Addr: ":" + httpPort, Handler: r}
	go func() {
		log.Printf("notification-service HTTP listening on :%s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
	log.Println("notification-service shutdown")
}
