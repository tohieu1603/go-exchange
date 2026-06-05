// @title           Auth Service API
// @version         1.0
// @description     Authentication, user management, KYC, API keys, referrals
// @host            localhost:8081
// @BasePath        /api

// Command server is the auth-service composition root: it loads config, opens the
// pgx pool + runs migrations, wires every use case over its pgx/sqlc adapter,
// starts the CQRS/event consumers and retention crons, and serves REST + gRPC.
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

	walletgrpc "github.com/cryptox/auth-service/internal/adapter/grpcclient"
	"github.com/cryptox/auth-service/internal/adapter/postgres"
	"github.com/cryptox/auth-service/internal/config"
	authgrpcsrv "github.com/cryptox/auth-service/internal/transport/grpc"
	httpiface "github.com/cryptox/auth-service/internal/transport/http"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/health"
	"github.com/cryptox/shared/httpx"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/proto/authpb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/sms"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/types"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/cryptox/auth-service/cmd/docs"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	tracingShutdown := tracing.Init("auth")
	defer tracingShutdown(context.Background())

	// ── Infrastructure ───────────────────────────────────────────────────────
	dsn := cfg.Dsn()
	pool, err := pgxdb.NewPool(ctx, dsn, cfg.MaxConns)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pgxdb.Migrate(dsn, cfg.MigrationsDir, "auth_schema_migrations"); err != nil {
		log.Printf("[auth] migrate: %v", err)
	}
	rdb := redisutil.MustClient(cfg.URL)

	// Repositories (pgx + sqlc).
	userRepo := postgres.NewUserRepo(pool)
	bonusRepo := postgres.NewBonusRepo(pool)
	rtRepo := postgres.NewRefreshTokenRepo(pool)
	apiKeyRepo := postgres.NewAPIKeyRepo(pool)
	feeTierRepo := postgres.NewFeeTierRepo(pool)
	referralRepo := postgres.NewReferralRepo(pool)
	auditRepo := postgres.NewAuditLogRepo(pool)
	kycRepo := postgres.NewKYCRepo(pool)
	fraudRepo := postgres.NewFraudRepo(pool)
	settingsRepo := postgres.NewSettingsRepo(pool)
	coinRepo := postgres.NewCoinRepo(pool)

	// Auth owns the coin reference catalog — seed once.
	if err := coinRepo.SeedDefaults(ctx); err != nil {
		log.Printf("[auth] seed coins: %v", err)
	}

	bus := eventbus.NewFromConfig(rdb, cfg.Brokers)

	// ── Services ─────────────────────────────────────────────────────────────
	mail := mailer.NewFromEnv()
	rtSvc := usecase.NewRefreshTokenService(rtRepo)
	auditSvc := usecase.NewAuditLogService(auditRepo)
	auditSvc.SetBus(bus) // publish audit.logged events for es-indexer
	throttle := usecase.NewLoginThrottle(rdb)
	smsSender := sms.NewFromEnv()
	stepUpSvc := usecase.NewStepUpService(rdb, mail, smsSender, userRepo, auditSvc)
	authSvc := usecase.NewAuthService(userRepo, rtSvc, auditSvc, throttle, stepUpSvc, mail, cfg.JWTSecret, rdb, bus)

	anomalySvc := usecase.NewAnomalyDetector(rdb, auditRepo, auditSvc, mail, userRepo)
	if cfg.ESURL != "" {
		anomalySvc.SetES(usecase.NewESAnomalyClient(cfg.ESURL))
	}
	authSvc.SetAnomalyDetector(anomalySvc)

	// Seed platform fee wallet (idempotent) and publish its ID to Redis so
	// trading/futures can resolve it at startup.
	if feeID, err := authSvc.EnsureFeeWallet(); err == nil && feeID > 0 {
		rdb.Set(context.Background(), types.RedisKeyFeeWalletID, feeID, 0)
	} else if err != nil {
		log.Printf("[auth] EnsureFeeWallet failed: %v", err)
	}
	authSvc.SeedAdmin() // after fee wallet so user IDs are predictable

	apiKeySvc := usecase.NewAPIKeyService(apiKeyRepo)
	feeTierSvc := usecase.NewFeeTierService(feeTierRepo, rdb)
	_ = feeTierSvc.SeedDefaults()

	walletClient := walletgrpc.NewWalletClient(cfg.WalletGRPCAddr)
	referralSvc := usecase.NewReferralService(referralRepo, walletClient, bus)

	adminSvc := usecase.NewAdminService(userRepo, kycRepo, rdb)
	kycSvc := usecase.NewKYCService(kycRepo, userRepo, rdb, mail)
	settingsSvc := usecase.NewSettingsService(settingsRepo, rdb)
	bonusSvc := usecase.NewBonusService(bonusRepo, userRepo, rdb)
	fraudSvc := usecase.NewFraudService(fraudRepo, userRepo, bonusSvc)

	// ── Handlers ─────────────────────────────────────────────────────────────
	authH := httpiface.NewAuthHandler(authSvc)
	apiKeyH := httpiface.NewAPIKeyHandler(apiKeySvc)
	feeTierH := httpiface.NewFeeTierHandler(feeTierSvc)
	referralH := httpiface.NewReferralHandler(referralSvc)
	auditH := httpiface.NewAuditLogHandler(auditSvc)
	adminH := httpiface.NewAdminHandler(adminSvc)
	kycH := httpiface.NewKYCHandler(kycSvc)
	settingsH := httpiface.NewSettingsHandler(settingsSvc)
	bonusH := httpiface.NewBonusHandler(bonusSvc)
	fraudH := httpiface.NewFraudHandler(fraudSvc)

	rl := redisutil.NewRateLimiter(rdb)

	r := gin.New()
	r.Use(gin.Logger(), middleware.Recovery(), middleware.CORS(), otelgin.Middleware("auth"), metrics.GinMiddleware("auth"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	health.New("auth").
		Register("postgres", pool.Ping).
		Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }).
		Mount(r)

	api := r.Group("/api")
	api.POST("/auth/register", middleware.RateLimit(rl, "register", time.Minute, 5), authH.Register)
	api.POST("/auth/login", middleware.RateLimit(rl, "login", time.Minute, 10), authH.Login)
	api.POST("/auth/google", middleware.RateLimit(rl, "google-login", time.Minute, 10), authH.GoogleLogin)
	api.POST("/auth/2fa/login", middleware.RateLimit(rl, "2fa-login", time.Minute, 5), authH.Login2FA)
	api.POST("/auth/step-up", middleware.RateLimit(rl, "stepup", time.Minute, 10), authH.StepUp)
	api.POST("/auth/refresh", middleware.RateLimit(rl, "refresh", time.Minute, 30), authH.RefreshAccessToken)
	api.GET("/fee-tiers", feeTierH.ListTiers) // public

	auth := api.Group("", middleware.JWTAuth(cfg.JWTSecret))
	auth.GET("/bonus/my", bonusH.MyBonus)
	auth.GET("/auth/profile", authH.Profile)
	auth.GET("/auth/ws-token", authH.WSToken)
	auth.PUT("/auth/profile", authH.UpdateProfile)
	auth.POST("/auth/change-password", authH.ChangePassword)
	auth.POST("/auth/set-password", authH.SetPassword)
	auth.POST("/auth/2fa/enable", authH.Enable2FA)
	auth.POST("/auth/2fa/verify", authH.Verify2FA)
	auth.POST("/auth/2fa/disable", authH.Disable2FA)
	auth.POST("/auth/logout", authH.LogoutHandler)
	auth.POST("/auth/logout-all", authH.LogoutAll)

	auth.GET("/api-keys", apiKeyH.List)
	auth.POST("/api-keys", apiKeyH.Create)
	auth.DELETE("/api-keys/:id", apiKeyH.Revoke)

	auth.GET("/auth/audit", auditH.MyAudit)
	auth.GET("/fee-tier/me", feeTierH.MyTier)

	auth.GET("/referral/code", referralH.MyCode)
	auth.GET("/referral/stats", referralH.MyStats)
	auth.GET("/referral/referees", referralH.MyReferees)
	auth.GET("/referral/commissions", referralH.MyCommissions)

	kyc := auth.Group("/kyc")
	kyc.POST("/email/send", kycH.SendVerifyEmail)
	kyc.POST("/email/verify", kycH.VerifyEmail)
	kyc.POST("/profile", kycH.SubmitProfile)
	kyc.POST("/document", kycH.UploadDocument)
	kyc.GET("/status", kycH.GetStatus)

	admin := auth.Group("/admin", middleware.AdminOnly())
	admin.GET("/users", adminH.Users)
	admin.GET("/users/:id", adminH.UserDetail)
	admin.GET("/users/:id/kyc", adminH.UserKycDetail)
	admin.PUT("/users/:id/kyc", adminH.UpdateKYC)
	admin.GET("/stats", adminH.Stats)
	admin.GET("/charts", adminH.Charts)
	admin.GET("/kyc/pending", kycH.ListPending)
	admin.POST("/kyc/:userId/approve", kycH.ApproveKYC)
	admin.POST("/kyc/:userId/reject", kycH.RejectKYC)
	admin.GET("/settings", settingsH.GetSettings)
	admin.PUT("/settings", settingsH.UpdateSettings)
	admin.POST("/bonus/promotions", bonusH.CreatePromotion)
	admin.GET("/bonus/promotions", bonusH.ListPromotions)
	admin.PUT("/bonus/promotions/:id/toggle", bonusH.TogglePromotion)
	admin.GET("/bonus/users/:userId", bonusH.UserBonus)
	admin.GET("/fraud", fraudH.ListFraudLogs)
	admin.PUT("/fraud/:id", fraudH.UpdateFraudAction)
	admin.POST("/users/:id/lock", fraudH.LockAccount)
	admin.POST("/users/:id/unlock", fraudH.UnlockAccount)
	admin.GET("/audit", auditH.AdminAudit)
	admin.StaticFS("/kyc-files", http.Dir("./uploads/kyc"))

	// ── Consumers ────────────────────────────────────────────────────────────
	// trade.executed → fraud detection + fee tier volume + referral commission.
	bus.Subscribe(eventbus.TopicTradeExecuted, func(_ context.Context, _ string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
		if err != nil {
			return nil
		}
		fraudSvc.OnTradeExecuted(event.BuyerID, event.SellerID, event.Pair, event.Amount, event.Total)
		if event.Side == "BUY" {
			_ = feeTierSvc.AddVolume(event.BuyerID, event.Total)
			referralSvc.OnTradeFee(event.BuyerID, event.BuyOrderID, "USDT", event.BuyerFee)
		} else {
			_ = feeTierSvc.AddVolume(event.SellerID, event.Total)
			referralSvc.OnTradeFee(event.SellerID, event.SellOrderID, "USDT", event.SellerFee)
		}
		return nil
	})
	// user.registered → bind referral.
	bus.Subscribe(eventbus.TopicUserRegistered, func(_ context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.UserRegisteredEvent](data)
		if err != nil {
			return nil
		}
		referralSvc.BindOnRegister(ev.UserID, ev.ReferralCode)
		return nil
	})
	// audit.request → persist via AuditLogService and re-publish audit.logged.
	bus.Subscribe(eventbus.TopicAuditRequest, func(_ context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.AuditRequestEvent](data)
		if err != nil {
			return nil
		}
		if ev.Outcome == "success" {
			auditSvc.Success(ev.UserID, ev.Email, ev.Action, ev.IP, "", ev.Detail)
		} else {
			auditSvc.Failure(ev.UserID, ev.Email, ev.Action, ev.IP, "", ev.Detail)
		}
		return nil
	})

	bus.StartConsumer(ctx, eventbus.TopicTradeExecuted, "auth-detector", "worker-1")
	bus.StartConsumer(ctx, eventbus.TopicUserRegistered, "auth-referral", "worker-1")
	bus.StartConsumer(ctx, eventbus.TopicAuditRequest, "auth-audit-ingest", "worker-1")
	log.Println("[auth] consumers started: trade.executed, user.registered, audit.request")

	// Periodic cleanup of expired refresh tokens.
	go func() {
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			authSvc.CleanupExpiredTokens()
			select {
			case <-t.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Audit log retention — daily prune.
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		if n := auditSvc.Prune(cfg.AuditRetentionDays); n > 0 {
			log.Printf("[audit] pruned %d rows older than %d days", n, cfg.AuditRetentionDays)
		}
		for {
			select {
			case <-t.C:
				if n := auditSvc.Prune(cfg.AuditRetentionDays); n > 0 {
					log.Printf("[audit] pruned %d rows older than %d days", n, cfg.AuditRetentionDays)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── gRPC server ──────────────────────────────────────────────────────────
	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			log.Fatalf("gRPC listen on %s: %v", cfg.GRPCPort, err)
		}
		grpcSrv := grpcutil.NewServer("auth")
		authpb.RegisterAuthServiceServer(grpcSrv, authgrpcsrv.NewAuthGRPCServer(authSvc, apiKeySvc, cfg.JWTSecret))
		log.Printf("gRPC server listening on :%s", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	srv := httpx.NewServer(":"+cfg.HTTPPort, r)
	go func() {
		log.Printf("HTTP server listening on :%s", cfg.HTTPPort)
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
	log.Println("auth-service shutdown")
}
