package usecase

import (
	"context"
	"time"

	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/shared/types"
)

// Persistence ports for the auth use-case layer.
//
// Clean architecture: these interfaces are owned by the use-case layer and the
// pgx/sqlc adapters in internal/adapter/postgres satisfy them structurally,
// injected from cmd/server. Every method takes a context.Context (cancellation +
// tracing); the auth use cases do not span multiple writes in a transaction, so
// no transaction handle leaks across this boundary.

// DailyCount is a date→count row for the admin growth chart.
type DailyCount struct {
	Date  string
	Count int64
}

// TradePairCounters are the post-update counters returned by the fraud
// trade-pair upsert, which the detector thresholds against.
type TradePairCounters struct {
	TradeCount int
	TotalVol   float64
	FirstTrade time.Time
}

type UserRepo interface {
	Create(ctx context.Context, user *domain.User) error
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByID(ctx context.Context, id uint) (*domain.User, error)
	FindByGoogleSub(ctx context.Context, sub string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateField(ctx context.Context, id uint, field string, value interface{}) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Count(ctx context.Context) int64
	FindPaginated(ctx context.Context, search string, page, size int) ([]domain.User, int64, error)
	UpdateKYC(ctx context.Context, id uint, status string) error
	// Admin read models.
	ListAdmin(ctx context.Context, search string, page, size int) ([]domain.User, int64, error)
	CountRealUsers(ctx context.Context) (int64, error)
	CountByKYCStatus(ctx context.Context, status string) (int64, error)
	UserGrowthDaily(ctx context.Context, since time.Time) ([]DailyCount, error)
}

type APIKeyRepo interface {
	Create(ctx context.Context, k *domain.APIKey) error
	FindByKeyID(ctx context.Context, keyID string) (*domain.APIKey, error)
	ListByUser(ctx context.Context, userID uint) ([]domain.APIKey, error)
	Revoke(ctx context.Context, id, userID uint) error
	UpdateLastUsed(ctx context.Context, id uint, ip string) error
}

type AuditLogRepo interface {
	Create(ctx context.Context, log *domain.AuditLog) error
	ListByUser(ctx context.Context, userID uint, page, size int) ([]domain.AuditLog, int64, error)
	ListAll(ctx context.Context, action string, page, size int) ([]domain.AuditLog, int64, error)
	PruneOlderThan(ctx context.Context, days int) int64
	HasDeviceForUser(ctx context.Context, userID uint, deviceID string) bool
}

type BonusRepo interface {
	CreatePromotion(ctx context.Context, promo *domain.BonusPromotion) error
	FindActivePromotions(ctx context.Context) ([]domain.BonusPromotion, error)
	FindAllPromotions(ctx context.Context) ([]domain.BonusPromotion, error)
	FindPromotionByID(ctx context.Context, id uint) (*domain.BonusPromotion, error)
	UpdatePromotion(ctx context.Context, promo *domain.BonusPromotion) error
	CreateUserBonus(ctx context.Context, bonus *domain.UserBonus) error
	FindUserBonuses(ctx context.Context, userID uint) ([]domain.UserBonus, error)
	FindActiveUserBonuses(ctx context.Context, userID uint) ([]domain.UserBonus, error)
	UpdateUserBonus(ctx context.Context, bonus *domain.UserBonus) error
	SumActiveBonus(ctx context.Context, userID uint) (float64, error)
}

type FeeTierRepo interface {
	ListAll(ctx context.Context) ([]domain.FeeTier, error)
	GetByLevel(ctx context.Context, level int) (*domain.FeeTier, error)
	GetUserVolume(ctx context.Context, userID uint) (*domain.UserVolume30d, error)
	UpsertVolume(ctx context.Context, userID uint, volume float64, level int) error
	IncrementVolume(ctx context.Context, userID uint, delta float64) error
	SeedDefaults(ctx context.Context) error
}

type KYCRepo interface {
	CreateProfile(ctx context.Context, profile *domain.KYCProfile) error
	FindProfileByUserID(ctx context.Context, userID uint) (*domain.KYCProfile, error)
	UpdateProfile(ctx context.Context, profile *domain.KYCProfile) error
	CreateDocument(ctx context.Context, doc *domain.KYCDocument) error
	FindDocumentsByUserID(ctx context.Context, userID uint) ([]domain.KYCDocument, error)
	UpdateDocumentStatus(ctx context.Context, docID uint, status, note string) error
	FindDocumentByUserAndType(ctx context.Context, userID uint, docType string) (*domain.KYCDocument, error)
	FindPendingUsers(ctx context.Context, page, size int) ([]domain.User, int64, error)
	UpdateAllDocumentsStatus(ctx context.Context, userID uint, status, note string) error
}

type ReferralRepo interface {
	CreateCode(ctx context.Context, c *domain.ReferralCode) error
	FindCodeByValue(ctx context.Context, code string) (*domain.ReferralCode, error)
	FindDefaultByUser(ctx context.Context, userID uint) (*domain.ReferralCode, error)
	IncrementUsage(ctx context.Context, codeID uint) error

	CreateReferral(ctx context.Context, r *domain.Referral) error
	FindReferralByReferee(ctx context.Context, refereeID uint) (*domain.Referral, error)
	ListReferees(ctx context.Context, referrerID uint, page, size int) ([]domain.Referral, int64, error)

	CreateCommission(ctx context.Context, c *domain.ReferralCommission) error
	FindCommissionByTrade(ctx context.Context, tradeID uint) (*domain.ReferralCommission, error)
	SumCommissionByUser(ctx context.Context, referrerID uint) (float64, error)
	ListCommissions(ctx context.Context, referrerID uint, page, size int) ([]domain.ReferralCommission, int64, error)
}

type RefreshTokenRepo interface {
	Create(ctx context.Context, rt *domain.RefreshToken) error
	FindByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	MarkUsed(ctx context.Context, id uint) error
	RevokeFamily(ctx context.Context, familyID, reason string) error
	RevokeByUser(ctx context.Context, userID uint, reason string) error
	RevokeByID(ctx context.Context, id uint, reason string) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// FraudRepo backs the anti-fraud service (fraud_logs + user_trade_pairs).
type FraudRepo interface {
	UpsertTradePair(ctx context.Context, u1, u2 uint, pair string, total float64) (TradePairCounters, error)
	CountActiveByTypeUsers(ctx context.Context, fraudType, userIDs string) (int64, error)
	CountByTypeUsers(ctx context.Context, fraudType, userIDs string) (int64, error)
	CreateFraudLog(ctx context.Context, l *domain.FraudLog) error
	ListFraudLogs(ctx context.Context, search string, page, size int) ([]domain.FraudLog, int64, error)
	UpdateFraudAction(ctx context.Context, logID uint, action, note string) error
}

// SettingsRepo backs the platform-settings single-row table.
type SettingsRepo interface {
	Get(ctx context.Context) (*types.PlatformSettings, error)
	Upsert(ctx context.Context, s *types.PlatformSettings) error
}
