package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/cryptox/auth-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

// AdminService backs the admin dashboard. Customer data comes from the user/KYC
// repositories; live counters come from Redis (event-projection driven) — see
// the consumer registrations in cmd/server.
type AdminService struct {
	users UserRepo
	kyc   KYCRepo
	rdb   *redis.Client
}

func NewAdminService(users UserRepo, kyc KYCRepo, rdb *redis.Client) *AdminService {
	return &AdminService{users: users, kyc: kyc, rdb: rdb}
}

func (s *AdminService) ctx() context.Context { return context.Background() }

func (s *AdminService) GetUsers(page, size int, search string) ([]domain.User, int64, error) {
	return s.users.ListAdmin(s.ctx(), search, page, size)
}

func (s *AdminService) GetUserByID(userID uint) (*domain.User, error) {
	return s.users.FindByID(s.ctx(), userID)
}

func (s *AdminService) GetUserKYCDetail(userID uint) (map[string]interface{}, error) {
	ctx := s.ctx()
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, _ := s.kyc.FindProfileByUserID(ctx, userID)
	docs, _ := s.kyc.FindDocumentsByUserID(ctx, userID)
	return map[string]interface{}{
		"kycStep":       user.KYCStep,
		"kycStatus":     user.KYCStatus,
		"emailVerified": user.EmailVerified,
		"profile":       profile,
		"documents":     docs,
	}, nil
}

func (s *AdminService) UpdateKYC(userID uint, status string) error {
	valid := map[string]bool{"NONE": true, "PENDING": true, "VERIFIED": true, "REJECTED": true}
	if !valid[status] {
		return errors.New("invalid KYC status")
	}
	return s.users.UpdateKYC(s.ctx(), userID, status)
}

// GetStats aggregates platform-wide statistics. Cross-service figures are read
// from Redis counters maintained by event consumers (database-per-service
// forbids cross-DB queries).
func (s *AdminService) GetStats() (map[string]interface{}, error) {
	totalUsers, err := s.users.CountRealUsers(s.ctx())
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"totalUsers":              totalUsers,
		"volume24h":               s.statRedisFloat("admin:stats:volume24h"),
		"activeOrders":            s.statRedisInt("admin:stats:active_orders"),
		"totalDeposited":          s.statRedisFloat("admin:stats:total_deposited"),
		"pendingWithdrawalsCount": s.statRedisInt("admin:stats:pending_withdrawals_count"),
		"pendingWithdrawalsSum":   s.statRedisFloat("admin:stats:pending_withdrawals_sum"),
		"activeFuturesPositions":  s.statRedisInt("admin:stats:active_futures"),
	}, nil
}

func (s *AdminService) statRedisFloat(key string) float64 {
	if s.rdb == nil {
		return 0
	}
	v, err := s.rdb.Get(s.ctx(), key).Float64()
	if err != nil {
		return 0
	}
	return v
}

func (s *AdminService) statRedisInt(key string) int64 {
	if s.rdb == nil {
		return 0
	}
	v, err := s.rdb.Get(s.ctx(), key).Int64()
	if err != nil {
		return 0
	}
	return v
}

// GetChartData returns daily aggregated data for the last 30 days.
func (s *AdminService) GetChartData() (map[string]interface{}, error) {
	ctx := s.ctx()
	since := time.Now().AddDate(0, 0, -30)
	rows, err := s.users.UserGrowthDaily(ctx, since)
	if err != nil {
		return nil, err
	}
	userGrowth := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		userGrowth[i] = map[string]interface{}{"date": r.Date, "count": r.Count}
	}
	pendingKyc, _ := s.users.CountByKYCStatus(ctx, "PENDING")
	return map[string]interface{}{
		"userGrowth":      userGrowth,
		"pendingKyc":      pendingKyc,
		"pendingDeposits": s.statRedisInt("admin:stats:pending_deposits"),
	}, nil
}
