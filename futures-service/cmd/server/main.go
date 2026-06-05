// @title           Futures Service API
// @version         1.0
// @description     Perpetual futures: positions, funding rates, TP/SL
// @host            localhost:8085
// @BasePath        /api

// Command server is the futures-service composition root: it loads config, opens
// the pgx pool + runs migrations, wires the position/liquidation/funding use
// cases over their adapters, starts the liquidation + funding loops, and the
// REST server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cryptox/futures-service/internal/adapter/grpcclient"
	"github.com/cryptox/futures-service/internal/adapter/postgres"
	"github.com/cryptox/futures-service/internal/adapter/pricesource"
	"github.com/cryptox/futures-service/internal/config"
	httpiface "github.com/cryptox/futures-service/internal/transport/http"
	"github.com/cryptox/futures-service/internal/usecase"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/futures-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("futures")
	defer tracingShutdown(context.Background())

	// ── Infrastructure ───────────────────────────────────────────────────────
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "futures_schema_migrations"); err != nil {
		log.Printf("[futures] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()
	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)

	walletClient := grpcclient.NewWalletClient(cfg.WalletGRPCAddr)
	priceSrc := pricesource.NewRedis(rdb)
	positionRepo := postgres.NewPositionRepo(pool)
	fundingRepo := postgres.NewFundingRepo(pool)
	txManager := postgres.NewTxManager(pool)

	// ── Application ──────────────────────────────────────────────────────────
	positionSvc := usecase.NewPositionService(positionRepo, walletClient, priceSrc, txManager, hub, bus)
	liquidationSvc := usecase.NewLiquidationService(positionRepo, walletClient, priceSrc, txManager, hub, bus)
	fundingSvc := usecase.NewFundingService(fundingRepo, positionRepo, walletClient, priceSrc, priceSrc, bus)

	liquidationSvc.Start(ctx)
	pairs := make([]string, 0, len(types.DefaultCoins))
	for _, coin := range types.DefaultCoins {
		pairs = append(pairs, coin.Symbol+"_USDT")
	}
	fundingSvc.Start(ctx, pairs)

	// ── Transport ────────────────────────────────────────────────────────────
	futuresH := httpiface.NewFuturesHandler(positionSvc)
	fundingH := httpiface.NewFundingHandler(fundingSvc)
	adminH := httpiface.NewAdminHandler(positionSvc, bus)

	r := gin.New()
	r.Use(middleware.Recovery(), otelgin.Middleware("futures"), metrics.GinMiddleware("futures"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	health.New("futures").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	pub := r.Group("/api/futures")
	pub.GET("/funding/:pair/latest", fundingH.Latest)
	pub.GET("/funding/:pair/history", fundingH.History)

	api := r.Group("/api/futures", middleware.JWTAuth(cfg.JWTSecret), middleware.KYCGate(rdb))
	api.POST("/order", middleware.RequireAPIPermission("trade"), futuresH.OpenPosition)
	api.POST("/close/:id", middleware.RequireAPIPermission("trade"), futuresH.ClosePosition)
	api.PUT("/positions/:id/tpsl", middleware.RequireAPIPermission("trade"), futuresH.UpdateTPSL)
	api.GET("/positions", futuresH.Positions)
	api.GET("/positions/open", futuresH.OpenPositions)
	api.GET("/funding/me", fundingH.MyHistory)

	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/positions", adminH.UserPositions)
		admin.POST("/users/:id/positions/:positionId/close", adminH.CloseUserPosition)
	}

	srv := httpx.NewServer(":"+cfg.HTTPPort, r)
	go func() {
		log.Printf("futures-service HTTP listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down futures-service...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Println("futures-service stopped")
}
