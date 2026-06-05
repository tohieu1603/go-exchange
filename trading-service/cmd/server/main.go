// @title           Trading Service API
// @version         1.0
// @description     Spot order placement, cancellation, and order history
// @host            localhost:8084
// @BasePath        /api

// Command server is the trading-service composition root: it loads config, opens
// the pgx pool + runs migrations, wires the matching engine and order use cases
// over their adapters, starts the CQRS projectors (trade.executed, order.updated)
// and the REST server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/trading-service/internal/adapter/feewallet"
	"github.com/cryptox/trading-service/internal/adapter/grpcclient"
	"github.com/cryptox/trading-service/internal/adapter/postgres"
	"github.com/cryptox/trading-service/internal/adapter/redisfee"
	"github.com/cryptox/trading-service/internal/config"
	httpiface "github.com/cryptox/trading-service/internal/transport/http"
	"github.com/cryptox/trading-service/internal/usecase"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/trading-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("trading")
	defer tracingShutdown(context.Background())

	// ── Infrastructure ───────────────────────────────────────────────────────
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "trading_schema_migrations"); err != nil {
		log.Printf("[trading] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)
	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)
	balCache := redisutil.NewBalanceCache(rdb)

	walletClient := grpcclient.NewWalletClient(cfg.WalletGRPCAddr)
	marketClient := grpcclient.NewMarketClient(cfg.MarketGRPCAddr)
	orderRepo := postgres.NewOrderRepo(pool)
	tradeRepo := postgres.NewTradeRepo(pool)
	feeResolver := redisfee.NewResolver(rdb, 0.001, 0.001) // VIP0 fallback
	go feewallet.Resolve(rdb, usecase.SetPlatformFeeUserID)

	// ── Application ──────────────────────────────────────────────────────────
	locker := usecase.NewBalanceLocker(balCache, bus)
	orderSvc := usecase.NewOrderService(orderRepo, locker)
	engine := usecase.NewMatchingEngine(orderRepo, balCache, locker, marketClient, bus, feeResolver)
	engine.LoadOpenOrders() // cold-start recovery into the in-memory book
	projector := usecase.NewProjector(orderRepo, tradeRepo)

	// CQRS projectors: this service owns orders + trades.
	bus.Subscribe(eventbus.TopicTradeExecuted, func(c context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
		if err != nil {
			return nil
		}
		return projector.ProjectTrade(c, ev)
	})
	bus.Subscribe(eventbus.TopicOrderUpdated, func(c context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.OrderEvent](data)
		if err != nil {
			return nil
		}
		return projector.ProjectOrder(c, ev)
	})
	bus.StartConsumer(ctx, eventbus.TopicTradeExecuted, "trade-projector", "worker-1")
	bus.StartConsumer(ctx, eventbus.TopicOrderUpdated, "order-projector", "worker-1")
	log.Println("[trading] CQRS projectors started (trade, order)")

	// ── Transport ────────────────────────────────────────────────────────────
	tradingH := httpiface.NewTradingHandler(engine, locker, walletClient, orderSvc)
	adminH := httpiface.NewAdminHandler(orderSvc, bus)

	r := gin.New()
	r.Use(middleware.Recovery(), otelgin.Middleware("trading"), metrics.GinMiddleware("trading"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	health.New("trading").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	api := r.Group("/api/trading", middleware.JWTAuth(cfg.JWTSecret), middleware.KYCGate(rdb))
	{
		api.POST("/orders", middleware.RequireAPIPermission("trade"), tradingH.PlaceOrder)
		api.DELETE("/orders/:id", middleware.RequireAPIPermission("trade"), tradingH.CancelOrder)
		api.GET("/orders", tradingH.OrderHistory)
		api.GET("/orders/open", tradingH.OpenOrders)
	}

	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/orders", adminH.UserOrders)
		admin.POST("/users/:id/orders/:orderId/cancel", adminH.CancelUserOrder)
	}

	srv := httpx.NewServer(":"+cfg.HTTPPort, r)
	go func() {
		log.Printf("trading-service listening on %s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down trading-service...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	log.Println("trading-service stopped")
}
