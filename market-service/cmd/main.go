// @title           Market Service API
// @version         1.0
// @description     Market data: tickers, order book depth, trades, candles, exchange rate
// @host            localhost:8083
// @BasePath        /api

package main

import (
	"context"
	"log"
	"net"
	"os"

	marketgrpc "github.com/cryptox/market-service/internal/grpc"
	"github.com/cryptox/market-service/internal/handler"
	"github.com/cryptox/market-service/internal/service"
	"github.com/cryptox/shared/config"
	"github.com/cryptox/market-service/internal/model"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/proto/marketpb"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/market-service/cmd/docs"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("market")
	defer tracingShutdown(context.Background())
	httpPort := getEnv("HTTP_PORT", "8083")
	grpcPort := getEnv("GRPC_PORT", "9083")

	// DB (for candles and trades read)
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}
	db.AutoMigrate(&model.Candle{})

	// Redis
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Redis URL parse error: %v", err)
	}
	rdb := redis.NewClient(opt)

	// WS Hub (broadcasts price/candle updates via Redis Pub/Sub)
	hub := ws.NewHub(rdb, cfg.JWTSecret)
	go hub.Run()

	// Services
	priceFeed := service.NewPriceFeed(rdb, hub, "")
	aggregator := service.NewCandleAggregator(db, hub, rdb)
	priceFeed.SetAggregator(aggregator)
	backfill := service.NewCandleBackfill(db, "")

	// VND rate refresh
	service.StartVNDRateRefresh()

	// HTTP handler
	marketHandler := handler.NewMarketHandler(priceFeed, aggregator, rdb)

	// Public HTTP routes (no auth needed)
	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware("market"), metrics.GinMiddleware("market"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/market")
	api.GET("/tickers", marketHandler.Tickers)
	api.GET("/depth/:pair", marketHandler.Depth)
	api.GET("/trades/:pair", marketHandler.Trades)
	api.GET("/candles/:pair", marketHandler.Candles)
	api.GET("/rate", marketHandler.ExchangeRate)

	// gRPC server
	grpcSrv := grpc.NewServer()
	marketpb.RegisterMarketServiceServer(grpcSrv, marketgrpc.NewMarketGRPCServer(priceFeed))

	go func() {
		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("gRPC listen error: %v", err)
		}
		log.Printf("gRPC server listening on :%s", grpcPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve error: %v", err)
		}
	}()

	// Background goroutines
	go priceFeed.Start()
	go backfill.Run()

	// CoinGecko-backed index price feed — populates index:{pair} keys for the
	// futures funding-rate scheduler to read.
	indexFeed := service.NewIndexFeed(rdb)
	indexFeed.Start(context.Background())

	log.Printf("market-service HTTP listening on :%s", httpPort)
	if err := r.Run(":" + httpPort); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
