// @title           Auth Service API
// @version         1.0
// @description     Authentication, user management, KYC, API keys, referrals
// @host            localhost:8081
// @BasePath        /api

package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	authgrpc "github.com/cryptox/auth-service/internal/grpc"
	"github.com/cryptox/auth-service/internal/handler"
	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/config"
	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/proto/authpb"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/sms"
	"github.com/cryptox/shared/types"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/cryptox/auth-service/cmd/docs"
)

func main() {
	cfg := config.LoadBase()
	tracingShutdown := tracing.Init("auth")
	defer tracingShutdown(context.Background())
	httpPort := getEnv("HTTP_PORT", "8081")
	grpcPort := getEnv("GRPC_PORT", "9081")

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	db.AutoMigrate(
		&model.User{},
		&model.KYCProfile{}, &model.KYCDocument{},
		&types.PlatformSettings{},
		&model.BonusPromotion{}, &model.UserBonus{},
		&model.FraudLog{}, &model.UserTradePair{},
		&model.RefreshToken{},
		&model.APIKey{},
		&model.FeeTier{}, &model.UserVolume30d{},
		&model.ReferralCode{}, &model.Referral{}, &model.ReferralCommission{},
		&model.AuditLog{},
	)
	// Coin reference data — auth-service owns the catalog table.
	db.AutoMigrate(&types.Coin{})
	seedCoins(db)

	rdb := newRedis(cfg.RedisURL)

	// Repos
	userRepo := repository.NewUserRepo(db)
	bonusRepo := repository.NewBonusRepo(db)
	rtRepo := repository.NewRefreshTokenRepo(db)
	apiKeyRepo := repository.NewAPIKeyRepo(db)
	feeTierRepo := repository.NewFeeTierRepo(db)
	referralRepo := repository.NewReferralRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)

	// EventBus
	var bus eventbus.EventBus
	if cfg.UseKafka() {
		bus = eventbus.NewKafkaBus(cfg.KafkaBrokerList(), rdb)
	} else {
		bus = eventbus.New(rdb)
	}

	// Services
	mail := mailer.NewFromEnv()
	rtSvc := service.NewRefreshTokenService(rtRepo)
	auditSvc := service.NewAuditLogService(auditRepo)
	auditSvc.SetBus(bus) // publish audit.logged events for es-indexer
	throttle := service.NewLoginThrottle(rdb)
	smsSender := sms.NewFromEnv()
	stepUpSvc := service.NewStepUpService(rdb, mail, smsSender, userRepo, auditSvc)
	authSvc := service.NewAuthService(userRepo, rtSvc, auditSvc, throttle, stepUpSvc, mail, cfg.JWTSecret, rdb, bus)

	// Anomaly detector — wired post-construction so it can also reuse auditRepo.
	anomalySvc := service.NewAnomalyDetector(rdb, auditRepo, auditSvc, mail, userRepo)
	if esURL := getEnv("ES_URL", "http://localhost:9201"); esURL != "" {
		anomalySvc.SetES(service.NewESAnomalyClient(esURL))
	}
	authSvc.SetAnomalyDetector(anomalySvc)
	authSvc.SetDB(db)

	// Seed platform fee wallet (idempotent) and publish its ID to Redis so
	// trading/futures can resolve it at startup.
	if feeID, err := authSvc.EnsureFeeWallet(); err == nil && feeID > 0 {
		rdb.Set(context.Background(), types.RedisKeyFeeWalletID, feeID, 0)
	} else if err != nil {
		log.Printf("[auth] EnsureFeeWallet failed: %v", err)
	}

	// Demo seed runs AFTER fee wallet so user IDs start at predictable values.
	authSvc.SeedAdmin()

	apiKeySvc := service.NewAPIKeyService(apiKeyRepo)
	feeTierSvc := service.NewFeeTierService(feeTierRepo, rdb)
	_ = feeTierSvc.SeedDefaults()

	// gRPC client to wallet-service (for immediate referral commission payout).
	walletGRPCAddr := getEnv("WALLET_GRPC_ADDR", "localhost:9082")
	walletClient := authgrpc.NewWalletClient(walletGRPCAddr)
	referralSvc := service.NewReferralService(referralRepo, walletClient, bus)

	adminSvc := service.NewAdminService(db, rdb)
	kycRepo := repository.NewKYCRepo(db)
	kycSvc := service.NewKYCService(kycRepo, userRepo, rdb, mail)
	settingsSvc := service.NewSettingsService(db, rdb)
	bonusSvc := service.NewBonusService(bonusRepo, userRepo, db, rdb)
	fraudSvc := service.NewFraudService(db, userRepo, bonusSvc)

	// Handlers
	authH := handler.NewAuthHandler(authSvc)
	apiKeyH := handler.NewAPIKeyHandler(apiKeySvc)
	feeTierH := handler.NewFeeTierHandler(feeTierSvc)
	referralH := handler.NewReferralHandler(referralSvc)
	auditH := handler.NewAuditLogHandler(auditSvc)
	adminH := handler.NewAdminHandler(adminSvc)
	kycH := handler.NewKYCHandler(kycSvc)
	settingsH := handler.NewSettingsHandler(settingsSvc)
	bonusH := handler.NewBonusHandler(bonusSvc)
	fraudH := handler.NewFraudHandler(fraudSvc)

	rl := redisutil.NewRateLimiter(rdb)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(), otelgin.Middleware("auth"), metrics.GinMiddleware("auth"), middleware.WAF())
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api")
	api.POST("/auth/register", rl.GinMiddleware("register", time.Minute, 5), authH.Register)
	api.POST("/auth/login", rl.GinMiddleware("login", time.Minute, 10), authH.Login)
	api.POST("/auth/2fa/login", rl.GinMiddleware("2fa-login", time.Minute, 5), authH.Login2FA)
	api.POST("/auth/step-up", rl.GinMiddleware("stepup", time.Minute, 10), authH.StepUp)
	api.POST("/auth/refresh", rl.GinMiddleware("refresh", time.Minute, 30), authH.RefreshAccessToken)
	api.GET("/fee-tiers", feeTierH.ListTiers) // public

	auth := api.Group("", middleware.JWTAuth(cfg.JWTSecret))
	auth.GET("/bonus/my", bonusH.MyBonus)
	auth.GET("/auth/profile", authH.Profile)
	auth.GET("/auth/ws-token", authH.WSToken)
	auth.PUT("/auth/profile", authH.UpdateProfile)
	auth.POST("/auth/change-password", authH.ChangePassword)
	auth.POST("/auth/2fa/enable", authH.Enable2FA)
	auth.POST("/auth/2fa/verify", authH.Verify2FA)
	auth.POST("/auth/2fa/disable", authH.Disable2FA)
	auth.POST("/auth/logout", authH.LogoutHandler)
	auth.POST("/auth/logout-all", authH.LogoutAll)

	// API key management (cookie-authed only — bot keys cannot create new bot keys)
	auth.GET("/api-keys", apiKeyH.List)
	auth.POST("/api-keys", apiKeyH.Create)
	auth.DELETE("/api-keys/:id", apiKeyH.Revoke)

	// Audit log — current user views own history; admins see global feed.
	auth.GET("/auth/audit", auditH.MyAudit)

	// Fee tier — current user
	auth.GET("/fee-tier/me", feeTierH.MyTier)

	// Referral
	auth.GET("/referral/code", referralH.MyCode)
	auth.GET("/referral/stats", referralH.MyStats)
	auth.GET("/referral/referees", referralH.MyReferees)
	auth.GET("/referral/commissions", referralH.MyCommissions)

	// KYC
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

	// Admin platform-wide audit feed.
	admin.GET("/audit", auditH.AdminAudit)

	// KYC document file serving — admins only. Files are saved by KYCHandler
	// to ./uploads/kyc/<userID>/<docType>.{jpg|png}; this route streams them
	// back so the admin UI can render previews. Path traversal is blocked by
	// gin's StaticFS (Clean()-d paths) plus the AdminOnly gate above.
	admin.StaticFS("/kyc-files", http.Dir("./uploads/kyc"))

	// Note: UserTradePair table is owned by market-service (auto-migrated there).

	// Consumer: trade.executed → fraud detection + fee tier volume + referral commission
	bus.Subscribe(eventbus.TopicTradeExecuted, func(_ context.Context, _ string, data []byte) error {
		event, err := eventbus.Unmarshal[eventbus.TradeEvent](data)
		if err != nil {
			return nil
		}
		fraudSvc.OnTradeExecuted(event.BuyerID, event.SellerID, event.Pair, event.Amount, event.Total)
		// Fee tier volume — count taker side (the aggressor) only, to avoid double-count.
		if event.Side == "BUY" {
			_ = feeTierSvc.AddVolume(event.BuyerID, event.Total)
			referralSvc.OnTradeFee(event.BuyerID, event.BuyOrderID, "USDT", event.BuyerFee)
		} else {
			_ = feeTierSvc.AddVolume(event.SellerID, event.Total)
			referralSvc.OnTradeFee(event.SellerID, event.SellOrderID, "USDT", event.SellerFee)
		}
		return nil
	})
	// Consumer: user.registered → bind referral
	bus.Subscribe(eventbus.TopicUserRegistered, func(_ context.Context, _ string, data []byte) error {
		ev, err := eventbus.Unmarshal[eventbus.UserRegisteredEvent](data)
		if err != nil {
			return nil
		}
		referralSvc.BindOnRegister(ev.UserID, ev.ReferralCode)
		return nil
	})
	// Consumer: audit.request → persist via AuditLogService. Other services
	// (wallet, futures, trading) emit on this topic when an admin acts on a
	// user; we own the audit_logs table and re-publish audit.logged for ES.
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

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	bus.StartConsumer(consumerCtx, eventbus.TopicTradeExecuted, "auth-detector", "worker-1")
	bus.StartConsumer(consumerCtx, eventbus.TopicUserRegistered, "auth-referral", "worker-1")
	bus.StartConsumer(consumerCtx, eventbus.TopicAuditRequest, "auth-audit-ingest", "worker-1")
	log.Println("[auth] consumers started: trade.executed, user.registered, audit.request")

	// Periodic cleanup of expired refresh tokens.
	go func() {
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			authSvc.CleanupExpiredTokens()
			select {
			case <-t.C:
			case <-consumerCtx.Done():
				return
			}
		}
	}()

	// Audit log retention — daily prune of rows older than AUDIT_RETENTION_DAYS (default 90).
	auditRetentionDays := 90
	if v := os.Getenv("AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			auditRetentionDays = n
		}
	}
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		// First run on boot so a service that's been down for >24h doesn't skip.
		if n := auditSvc.Prune(auditRetentionDays); n > 0 {
			log.Printf("[audit] pruned %d rows older than %d days", n, auditRetentionDays)
		}
		for {
			select {
			case <-t.C:
				if n := auditSvc.Prune(auditRetentionDays); n > 0 {
					log.Printf("[audit] pruned %d rows older than %d days", n, auditRetentionDays)
				}
			case <-consumerCtx.Done():
				return
			}
		}
	}()

	// gRPC server
	go func() {
		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("gRPC listen on %s: %v", grpcPort, err)
		}
		grpcSrv := grpc.NewServer()
		authpb.RegisterAuthServiceServer(grpcSrv, authgrpc.NewAuthGRPCServer(authSvc, apiKeySvc, cfg.JWTSecret))
		log.Printf("gRPC server listening on :%s", grpcPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	srv := &http.Server{Addr: ":" + httpPort, Handler: r}
	go func() {
		log.Printf("HTTP server listening on :%s", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("auth-service shutdown")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newRedis(redisURL string) *redis.Client {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("redis parse URL: %v", err)
	}
	return redis.NewClient(opt)
}

func seedCoins(db *gorm.DB) {
	var count int64
	db.Model(&types.Coin{}).Count(&count)
	if count > 0 {
		return
	}
	db.CreateInBatches(types.DefaultCoins, 20)
}
