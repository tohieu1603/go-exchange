// @title           Wallet Service API
// @version         1.0
// @description     Wallet balances, deposits, withdrawals
// @host            localhost:8082
// @BasePath        /api

// Command server is the wallet-service composition root: it loads config, opens
// the pgx pool + runs migrations, wires the use case over its pgx/sqlc adapters,
// starts the CQRS projectors (balance.changed, user.registered) and the REST +
// gRPC servers.
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

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/proto/walletpb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/wallet-service/internal/adapter/grpcclient"
	"github.com/cryptox/wallet-service/internal/adapter/postgres"
	"github.com/cryptox/wallet-service/internal/adapter/rate"
	"github.com/cryptox/wallet-service/internal/adapter/settings"
	"github.com/cryptox/wallet-service/internal/config"
	grpciface "github.com/cryptox/wallet-service/internal/transport/grpc"
	httpiface "github.com/cryptox/wallet-service/internal/transport/http"
	"github.com/cryptox/wallet-service/internal/usecase"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/wallet-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("wallet")
	defer tracingShutdown(context.Background())

	// ── Infrastructure ───────────────────────────────────────────────────────
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "wallet_schema_migrations"); err != nil {
		log.Printf("[wallet] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)
	balCache := redisutil.NewBalanceCache(rdb)
	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)

	rateProvider := rate.NewProvider()
	rateProvider.Start(ctx)

	walletRepo := postgres.NewWalletRepo(pool)
	depositRepo := postgres.NewDepositRepo(pool)
	withdrawalRepo := postgres.NewWithdrawalRepo(pool)
	txManager := postgres.NewTxManager(pool)
	settingsGate := settings.NewRedisGate(rdb)
	authClient := grpcclient.NewAuthClient(cfg.AuthGRPCAddr)

	sepayCfg := usecase.SepayConfig{
		BankCode:    cfg.SepayBankCode,
		BankAccount: cfg.SepayBankAccount,
		AccountName: cfg.SepayAccountName,
	}

	// ── Application ──────────────────────────────────────────────────────────
	uc := usecase.NewWalletUseCase(walletRepo, depositRepo, withdrawalRepo, txManager, balCache, settingsGate, rateProvider, bus, sepayCfg)

	// Cold-start: warm Redis from DB without clobbering live balances.
	go func() {
		if n, err := uc.WarmCache(ctx); err == nil {
			log.Printf("[wallet] cold-start warmed %d wallets into Redis", n)
		}
	}()

	// CQRS projector: owns the wallets table.
	bus.Subscribe(eventbus.TopicBalanceChanged, func(c context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.BalanceEvent](data)
		if err != nil {
			return nil
		}
		return uc.ApplyBalanceChange(c, ev.UserID, ev.Currency, ev.Delta, ev.Reason)
	})
	bus.StartConsumer(ctx, eventbus.TopicBalanceChanged, "balance-projector", "worker-1")

	// Provision wallets for newly-registered users.
	bus.Subscribe(eventbus.TopicUserRegistered, func(c context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.UserRegisteredEvent](data)
		if err != nil {
			return nil
		}
		return uc.ProvisionWallets(c, ev.UserID, ev.SeedProfile)
	})
	bus.StartConsumer(ctx, eventbus.TopicUserRegistered, "wallet-user-projector", "worker-1")
	log.Println("[wallet] CQRS projectors started (balance, user-registered)")

	// ── Transport ────────────────────────────────────────────────────────────
	walletH := httpiface.NewWalletHandler(uc, authClient)
	webhookH := httpiface.NewWebhookHandler(uc, cfg.SepayAPIKey, cfg.SepaySecretKey)
	adminH := httpiface.NewAdminWalletHandler(uc, bus)

	r := gin.New()
	r.Use(middleware.Recovery(), otelgin.Middleware("wallet"), metrics.GinMiddleware("wallet"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Liveness/readiness for orchestrators and load balancers.
	health.New("wallet").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	auth := r.Group("/api/wallet", middleware.JWTAuth(cfg.JWTSecret))
	{
		auth.GET("/balances", walletH.Balances)
		auth.POST("/deposit", walletH.CreateDeposit)
		auth.GET("/deposits", walletH.Deposits)
		auth.POST("/withdraw", middleware.RequireAPIPermission("withdraw"), walletH.Withdraw)
		auth.GET("/withdrawals", walletH.Withdrawals)
	}

	admin := r.Group("/api/admin", middleware.JWTAuth(cfg.JWTSecret), middleware.AdminOnly())
	{
		admin.GET("/users/:id/wallets", adminH.UserWallets)
		admin.POST("/users/:id/wallets/:currency/adjust", adminH.AdjustBalance)
		admin.POST("/users/:id/wallets/adjust-batch", adminH.AdjustBalanceBatch)
		admin.GET("/deposits", adminH.Deposits)
		admin.POST("/deposits/:id/confirm", adminH.ConfirmDeposit)
		admin.POST("/deposits/:id/reject", adminH.RejectDeposit)
		admin.GET("/withdrawals", adminH.Withdrawals)
		admin.POST("/withdrawals/:id/approve", adminH.ApproveWithdrawal)
		admin.POST("/withdrawals/:id/reject", adminH.RejectWithdrawal)
	}

	r.POST("/api/webhook/sepay", webhookH.SepayCallback)
	r.GET("/api/market/rate", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"rate": rateProvider.VNDRate(), "base": "USD", "target": "VND"})
	})

	httpSrv := httpx.NewServer(":"+cfg.HTTPPort, r)

	// gRPC server (panic recovery + error-mapping + timeout interceptors).
	grpcSrv := grpcutil.NewServer("wallet")
	walletpb.RegisterWalletServiceServer(grpcSrv, grpciface.NewServer(uc))
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen error: %v", err)
	}

	go func() {
		log.Printf("[wallet-service] gRPC listening on :%s", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Printf("[wallet-service] gRPC error: %v", err)
		}
	}()
	go func() {
		log.Printf("[wallet-service] HTTP listening on :%s", cfg.HTTPPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[wallet-service] HTTP error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[wallet-service] shutting down...")
	grpcSrv.GracefulStop()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
	log.Println("[wallet-service] stopped")
}
