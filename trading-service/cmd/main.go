// @title           Trading Service API
// @version         1.0
// @description     Spot order placement, cancellation, and order history
// @host            localhost:8084
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

	"github.com/cryptox/shared/config"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/trading-service/internal/model"
	"github.com/cryptox/shared/redisutil"
	grpcclient "github.com/cryptox/trading-service/internal/grpc"
	"github.com/cryptox/trading-service/internal/handler"
	"github.com/cryptox/trading-service/internal/repository"
	"github.com/cryptox/trading-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/trading-service/cmd/docs"
)

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("trading")
	defer tracingShutdown(context.Background())
	walletGRPCAddr := getEnv("WALLET_GRPC_ADDR", "localhost:9082")
	marketGRPCAddr := getEnv("MARKET_GRPC_ADDR", "localhost:9083")
	httpPort := getEnv("HTTP_PORT", "8084")

	// DB
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}
	db.AutoMigrate(&model.Order{}, &model.Trade{})

	// Redis
	opt, _ := redis.ParseURL(cfg.RedisURL)
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connect error: %v", err)
	}

	// Event bus
	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	// BalanceCache: direct Redis Lua for hot trading path (no gRPC on critical path)
	balCache := redisutil.NewBalanceCache(rdb)

	// gRPC clients
	walletClient := grpcclient.NewWalletClient(walletGRPCAddr)
	marketClient := grpcclient.NewMarketClient(marketGRPCAddr)

	// Repository
	orderRepo := repository.NewOrderRepo(db)

	// Services
	locker := service.NewBalanceLocker(balCache, bus)
	orderSvc := service.NewOrderService(orderRepo, balCache, locker)
	feeResolver := service.NewRedisFeeResolver(rdb, 0.001, 0.001) // VIP0 fallback
	// Resolve platform fee wallet user ID from Redis (auth-service publishes it).
	go service.ResolveFeeWalletID(rdb)
	matchingEngine := service.NewMatchingEngine(orderRepo, balCache, locker, walletClient, marketClient, bus, feeResolver)

	// Cold-start recovery: load open LIMIT orders into in-memory orderbook
	matchingEngine.LoadOpenOrders()

	// CQRS projectors: this service owns orders + trades tables
	bus.Subscribe(eventbus.TopicTradeExecuted, func(_ context.Context, id string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
		if err != nil {
			return nil
		}
		return db.Create(&model.Trade{
			Pair: event.Pair, BuyOrderID: event.BuyOrderID, SellOrderID: event.SellOrderID,
			BuyerID: event.BuyerID, SellerID: event.SellerID,
			Price: event.Price, Amount: event.Amount, Total: event.Total,
			BuyerFee: event.BuyerFee, SellerFee: event.SellerFee,
		}).Error
	})
	bus.Subscribe(eventbus.TopicOrderUpdated, func(_ context.Context, id string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.OrderEvent](data)
		if err != nil {
			return nil
		}
		return db.Exec(
			"UPDATE orders SET status=?, filled_amount=?, price=CASE WHEN price=0 THEN ? ELSE price END, updated_at=NOW() WHERE id=?",
			event.Status, event.FilledAmount, event.Price, event.OrderID,
		).Error
	})
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	bus.StartConsumer(consumerCtx, eventbus.TopicTradeExecuted, "trade-projector", "worker-1")
	bus.StartConsumer(consumerCtx, eventbus.TopicOrderUpdated, "order-projector", "worker-1")
	log.Println("[trading] CQRS projectors started (trade, order)")

	// Handlers
	tradingHandler := handler.NewTradingHandler(matchingEngine, balCache, locker, walletClient, orderSvc)
	adminHandler := handler.NewAdminHandler(orderSvc, bus)

	// HTTP router
	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware("trading"), metrics.GinMiddleware("trading"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/trading", middleware.JWTAuth(cfg.JWTSecret), middleware.KYCGate(rdb))
	{
		// Mutating routes — API-key callers must hold "trade" permission.
		api.POST("/orders", middleware.RequireAPIPermission("trade"), tradingHandler.PlaceOrder)
		api.DELETE("/orders/:id", middleware.RequireAPIPermission("trade"), tradingHandler.CancelOrder)
		// Read-only — read permission implicit (any active key).
		api.GET("/orders", tradingHandler.OrderHistory)
		api.GET("/orders/open", tradingHandler.OpenOrders)
	}

	// Admin — view any user's order history. Gateway routes
	// /api/admin/users/:id/orders to this service via a special-case match.
	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/orders", adminHandler.UserOrders)
		admin.POST("/users/:id/orders/:orderId/cancel", adminHandler.CancelUserOrder)
	}

	srv := &http.Server{Addr: ":" + httpPort, Handler: r}

	go func() {
		log.Printf("trading-service listening on %s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down trading-service...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	log.Println("trading-service stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
