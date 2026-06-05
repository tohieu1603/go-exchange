// @title           Market Service API
// @version         1.0
// @description     Market data: tickers, order book depth, trades, candles, exchange rate
// @host            localhost:8083
// @BasePath        /api

// Command server is the market-service composition root: it loads config, opens
// the pgx pool + runs migrations, wires the candle aggregator/query and market
// reads over their adapters, starts the price/index feeds + backfill, and the
// REST + gRPC servers.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cryptox/market-service/internal/adapter/cache"
	"github.com/cryptox/market-service/internal/adapter/feed"
	"github.com/cryptox/market-service/internal/adapter/marketstore"
	"github.com/cryptox/market-service/internal/adapter/postgres"
	"github.com/cryptox/market-service/internal/adapter/rate"
	"github.com/cryptox/market-service/internal/config"
	grpciface "github.com/cryptox/market-service/internal/transport/grpc"
	httpiface "github.com/cryptox/market-service/internal/transport/http"
	"github.com/cryptox/market-service/internal/usecase"
	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/proto/marketpb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/market-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("market")
	defer tracingShutdown(context.Background())

	// ── Infrastructure ───────────────────────────────────────────────────────
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "market_schema_migrations"); err != nil {
		log.Printf("[market] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()

	candleRepo := postgres.NewCandleRepo(pool)
	candleCache := cache.NewCandleCache(rdb)
	store := marketstore.NewStore(rdb)
	rateProvider := rate.NewProvider()
	rateProvider.Start(ctx)

	// ── Application ──────────────────────────────────────────────────────────
	aggregator := usecase.NewCandleAggregator(candleRepo, candleCache, hub)
	candleQuery := usecase.NewCandleQuery(candleRepo, candleCache)
	marketQuery := usecase.NewMarketQuery(store)

	// External feeds (infrastructure adapters)
	priceFeed := feed.NewPriceFeed(rdb, hub, aggregator)
	go priceFeed.Start()
	go feed.NewBackfill(candleRepo).Run()
	feed.NewIndexFeed(rdb).Start(ctx)

	// ── Transport ────────────────────────────────────────────────────────────
	h := httpiface.NewHandler(candleQuery, marketQuery, rateProvider)
	r := gin.New()
	r.Use(middleware.Recovery(), otelgin.Middleware("market"), metrics.GinMiddleware("market"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	health.New("market").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	api := r.Group("/api/market")
	api.GET("/tickers", h.Tickers)
	api.GET("/depth/:pair", h.Depth)
	api.GET("/trades/:pair", h.Trades)
	api.GET("/candles/:pair", h.Candles)
	api.GET("/rate", h.ExchangeRate)

	grpcSrv := grpcutil.NewServer("market")
	marketpb.RegisterMarketServiceServer(grpcSrv, grpciface.NewServer(marketQuery))
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen error: %v", err)
	}
	go func() {
		log.Printf("market-service gRPC listening on :%s", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Printf("gRPC serve error: %v", err)
		}
	}()

	srv := httpx.NewServer(":"+cfg.HTTPPort, r)
	go func() {
		log.Printf("market-service HTTP listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("market-service shutting down...")
	grpcSrv.GracefulStop()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	log.Println("market-service stopped")
}
