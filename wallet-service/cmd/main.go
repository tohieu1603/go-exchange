// @title           Wallet Service API
// @version         1.0
// @description     Wallet balances, deposits, withdrawals
// @host            localhost:8082
// @BasePath        /api

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

	"github.com/cryptox/shared/config"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/wallet-service/internal/model"
	"github.com/cryptox/shared/proto/walletpb"
	"github.com/cryptox/shared/redisutil"
	walletgrpc "github.com/cryptox/wallet-service/internal/grpc"
	"github.com/cryptox/wallet-service/internal/handler"
	"github.com/cryptox/wallet-service/internal/repository"
	"github.com/cryptox/wallet-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/wallet-service/cmd/docs"
)

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("wallet")
	defer tracingShutdown(context.Background())
	httpPort := getEnv("HTTP_PORT", "8082")
	grpcPort := getEnv("GRPC_PORT", "9082")

	// DB
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}

	// Redis
	rdbOpt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Redis URL parse error: %v", err)
	}
	rdb := redis.NewClient(rdbOpt)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis ping error: %v", err)
	}

	// AutoMigrate tables owned by this service
	db.AutoMigrate(&model.Wallet{}, &model.Deposit{}, &model.Withdrawal{})

	// EventBus
	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	// Repos
	walletRepo := repository.NewWalletRepo(db)
	depositRepo := repository.NewDepositRepo(db)
	withdrawalRepo := repository.NewWithdrawalRepo(db)

	// SePay config
	sepay := service.SepayConfig{
		BankCode:    getEnv("SEPAY_BANK_CODE", ""),
		BankAccount: getEnv("SEPAY_BANK_ACCOUNT", ""),
		AccountName: getEnv("SEPAY_ACCOUNT_NAME", ""),
	}

	// Wallet service
	walletSvc := service.NewWalletService(walletRepo, depositRepo, withdrawalRepo, db, rdb, sepay, bus)

	// Cold-start: warm Redis with the canonical (balance, locked) for every
	// wallet so trading-service's hot-path Lua reads match DB on first request.
	// WarmIfMissing protects against clobbering live state if Redis stayed up.
	go func() {
		var rows []model.Wallet
		if err := db.Find(&rows).Error; err == nil {
			balCache := redisutil.NewBalanceCache(rdb)
			for _, w := range rows {
				balCache.WarmIfMissing(ctx, w.UserID, w.Currency, w.Balance, w.LockedBalance)
			}
			log.Printf("[wallet] cold-start warmed %d wallets into Redis", len(rows))
		}
	}()

	// NOTE: admin/demo wallets are seeded via the user.registered consumer
	// below (SeedProfile field on the event). Auth-service no longer touches
	// our DB — clean database-per-service boundary.

	// CQRS projector: owns wallets table.
	//
	// reason routing:
	//   "lock"   → +locked_balance (no balance change)
	//   "unlock" → -locked_balance
	//   else     → balance += delta (signed)
	//
	// "trade", "fee", "funding", "liquidation", "grpc" all map to the
	// default branch since they are real ledger movements.
	bus.Subscribe(eventbus.TopicBalanceChanged, func(_ context.Context, id string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.BalanceEvent](data)
		if err != nil {
			return nil
		}
		switch event.Reason {
		case "lock":
			return db.Exec("UPDATE wallets SET locked_balance=locked_balance+?, updated_at=NOW() WHERE user_id=? AND currency=?",
				event.Delta, event.UserID, event.Currency).Error
		case "unlock":
			return db.Exec("UPDATE wallets SET locked_balance=GREATEST(locked_balance-?, 0), updated_at=NOW() WHERE user_id=? AND currency=?",
				event.Delta, event.UserID, event.Currency).Error
		}
		// Real delta to balance.
		if event.Delta < 0 {
			return db.Exec("UPDATE wallets SET balance=balance+?, updated_at=NOW() WHERE user_id=? AND currency=? AND balance>=?",
				event.Delta, event.UserID, event.Currency, -event.Delta).Error
		}
		return db.Exec("UPDATE wallets SET balance=balance+?, updated_at=NOW() WHERE user_id=? AND currency=?",
			event.Delta, event.UserID, event.Currency).Error
	})
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	bus.StartConsumer(consumerCtx, eventbus.TopicBalanceChanged, "balance-projector", "worker-1")

	// User-registered consumer: provisions wallets for every new user.
	// Honors SeedProfile field for special accounts:
	//   "system"  → empty wallets (fee_wallet collects but starts at 0)
	//   "admin"   → demo balances for admin testing
	//   "demo"    → mid-size demo balances for end-user demo
	//   ""/other  → default 10K USDT signup gift
	bus.Subscribe(eventbus.TopicUserRegistered, func(_ context.Context, id string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.UserRegisteredEvent](data)
		if err != nil {
			return nil
		}
		// Skip if wallets already exist for this user (consumer replay safety).
		var existing int64
		db.Model(&model.Wallet{}).Where("user_id = ?", event.UserID).Count(&existing)
		if existing > 0 {
			return nil
		}

		log.Printf("[wallet] provisioning wallets user=%d profile=%q", event.UserID, event.SeedProfile)

		vnd, usdt, btc, eth := signupBalances(event.SeedProfile)
		wallets := []model.Wallet{
			{UserID: event.UserID, Currency: "VND", Balance: vnd},
			{UserID: event.UserID, Currency: "USDT", Balance: usdt},
		}
		for _, coin := range types.DefaultCoins {
			bal := 0.0
			if event.SeedProfile == "admin" && coin.Symbol == "BTC" {
				bal = btc
			}
			if event.SeedProfile == "admin" && coin.Symbol == "ETH" {
				bal = eth
			}
			wallets = append(wallets, model.Wallet{UserID: event.UserID, Currency: coin.Symbol, Balance: bal})
		}
		return db.CreateInBatches(wallets, 50).Error
	})
	bus.StartConsumer(consumerCtx, eventbus.TopicUserRegistered, "wallet-user-projector", "worker-1")

	log.Println("[wallet] CQRS projectors started (balance, user-registered)")

	// gRPC client to auth-service for 2FA verification on withdrawals.
	authGRPCAddr := getEnv("AUTH_GRPC_ADDR", "localhost:9081")
	authClient := walletgrpc.NewAuthClient(authGRPCAddr)

	// Handlers
	walletH := handler.NewWalletHandler(walletSvc, authClient)
	webhookH := handler.NewWebhookHandler(walletSvc,
		getEnv("SEPAY_API_KEY", ""),
		getEnv("SEPAY_SECRET_KEY", ""),
	)
	adminH := handler.NewAdminWalletHandler(walletSvc, bus)

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware("wallet"), metrics.GinMiddleware("wallet"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	auth := r.Group("/api/wallet", middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/balances", walletH.Balances)
		auth.POST("/deposit", walletH.CreateDeposit)
		auth.GET("/deposits", walletH.Deposits)
		// Withdraw — requires "withdraw" permission for API-key callers.
		// Cookie/browser callers pass through (full-trust session).
		auth.POST("/withdraw", middleware.RequireAPIPermission("withdraw"), walletH.Withdraw)
		auth.GET("/withdrawals", walletH.Withdrawals)
	}

	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/wallets", adminH.UserWallets)
		admin.POST("/users/:id/wallets/:currency/adjust", adminH.AdjustBalance)
		admin.GET("/deposits", adminH.Deposits)
		admin.POST("/deposits/:id/confirm", adminH.ConfirmDeposit)
		admin.POST("/deposits/:id/reject", adminH.RejectDeposit)
		admin.GET("/withdrawals", adminH.Withdrawals)
		admin.POST("/withdrawals/:id/approve", adminH.ApproveWithdrawal)
		admin.POST("/withdrawals/:id/reject", adminH.RejectWithdrawal)
	}

	r.POST("/api/webhook/sepay", webhookH.SepayCallback)
	r.GET("/api/market/rate", func(c *gin.Context) {
		c.JSON(200, gin.H{"rate": service.GetVNDRate(), "base": "USD", "target": "VND"})
	})

	httpSrv := &http.Server{Addr: ":" + httpPort, Handler: r}

	// gRPC server
	grpcSrv := grpc.NewServer()
	walletpb.RegisterWalletServiceServer(grpcSrv, walletgrpc.NewWalletGRPCServer(walletSvc))

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("gRPC listen error: %v", err)
	}

	go func() {
		log.Printf("[wallet-service] gRPC listening on :%s", grpcPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Printf("[wallet-service] gRPC error: %v", err)
		}
	}()

	go func() {
		log.Printf("[wallet-service] HTTP listening on :%s", httpPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[wallet-service] HTTP error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[wallet-service] shutting down...")
	grpcSrv.GracefulStop()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutCtx)
	log.Println("[wallet-service] stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// signupBalances returns initial wallet balances per SeedProfile hint
// carried on the user.registered event. Defaults to a small USDT gift.
//
// Returns: (vnd, usdt, btc, eth)
func signupBalances(profile string) (float64, float64, float64, float64) {
	switch profile {
	case "system":
		return 0, 0, 0, 0
	case "admin":
		return 100_000_000, 100_000, 1.0, 10.0
	case "demo":
		return 50_000_000, 10_000, 0, 0
	default:
		return 0, 10_000, 0, 0
	}
}
