// @title           Futures Service API
// @version         1.0
// @description     Perpetual futures: positions, funding rates, TP/SL
// @host            localhost:8085
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

	svcgrpc "github.com/cryptox/futures-service/internal/grpc"
	"github.com/cryptox/futures-service/internal/handler"
	"github.com/cryptox/futures-service/internal/repository"
	"github.com/cryptox/futures-service/internal/service"
	"github.com/cryptox/shared/config"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/futures-service/internal/model"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/futures-service/cmd/docs"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("futures")
	defer tracingShutdown(context.Background())
	httpPort := getEnv("HTTP_PORT", "8085")
	walletGRPCAddr := getEnv("WALLET_GRPC_ADDR", "localhost:9082")
	marketGRPCAddr := getEnv("MARKET_GRPC_ADDR", "localhost:9083")

	// DB
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}
	db.AutoMigrate(&model.FuturesPosition{}, &model.FundingRate{}, &model.FundingPayment{})

	// Redis
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Redis URL parse error: %v", err)
	}
	rdb := redis.NewClient(opt)

	// WS Hub (Redis Pub/Sub for position/liquidation broadcasts)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()

	// EventBus (Redis Streams)
	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	// gRPC clients
	walletClient := svcgrpc.NewWalletClient(walletGRPCAddr)
	_ = svcgrpc.NewMarketClient(marketGRPCAddr) // available if needed

	// Repos — only positions + funding rows live in pg-futures.
	// Wallet operations go via gRPC to wallet-service.
	positionRepo := repository.NewPositionRepo(db)
	fundingRepo := repository.NewFundingRepo(db)

	// Services
	futuresSvc := service.NewFuturesService(positionRepo, walletClient, db, rdb, hub, bus)
	liquidationEngine := service.NewLiquidationEngine(positionRepo, walletClient, db, rdb, hub, bus)
	fundingSvc := service.NewFundingService(fundingRepo, positionRepo, walletClient, rdb, bus)

	// Handlers
	futuresHandler := handler.NewFuturesHandler(futuresSvc)
	fundingHandler := handler.NewFundingHandler(fundingSvc)
	adminHandler := handler.NewAdminHandler(futuresSvc)

	// CQRS projector: this service owns futures_positions table
	bus.Subscribe(eventbus.TopicPositionChanged, func(_ context.Context, id string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.PositionEvent](data)
		if err != nil {
			return nil
		}
		return db.Exec("UPDATE futures_positions SET status=?, mark_price=?, unrealized_pn_l=? WHERE id=?",
			event.Status, event.MarkPrice, event.PnL, event.PositionID).Error
	})
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	bus.StartConsumer(consumerCtx, eventbus.TopicPositionChanged, "position-projector", "worker-1")
	log.Println("[futures] CQRS position projector started")

	// Start liquidation engine
	liquidationEngine.Start()

	// Start funding scheduler — settles every 8h across all default perpetual pairs.
	pairs := make([]string, 0, len(types.DefaultCoins))
	for _, coin := range types.DefaultCoins {
		pairs = append(pairs, coin.Symbol+"_USDT")
	}
	fundingSvc.Start(consumerCtx, pairs)

	// HTTP routes
	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware("futures"), metrics.GinMiddleware("futures"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Public funding rate endpoints — anyone can read.
	pub := r.Group("/api/futures")
	pub.GET("/funding/:pair/latest", fundingHandler.Latest)
	pub.GET("/funding/:pair/history", fundingHandler.History)

	api := r.Group("/api/futures", middleware.JWTAuth(cfg.JWTSecret), middleware.KYCGate(rdb))
	// Mutating routes require "trade" permission for API-key callers.
	api.POST("/order", middleware.RequireAPIPermission("trade"), futuresHandler.OpenPosition)
	api.POST("/close/:id", middleware.RequireAPIPermission("trade"), futuresHandler.ClosePosition)
	api.PUT("/positions/:id/tpsl", middleware.RequireAPIPermission("trade"), futuresHandler.UpdateTPSL)
	api.GET("/positions", futuresHandler.Positions)
	api.GET("/positions/open", futuresHandler.OpenPositions)
	api.GET("/funding/me", fundingHandler.MyHistory)

	// Admin — view any user's positions. Gateway routes
	// /api/admin/users/:id/positions to this service via a special-case match.
	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/positions", adminHandler.UserPositions)
	}

	srv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: r,
	}

	go func() {
		log.Printf("futures-service HTTP listening on :%s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down futures-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Println("futures-service stopped")
}
