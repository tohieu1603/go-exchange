package service

import (
	"context"
	"errors"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// AdminService backs the admin dashboard. Stats come from Redis counters
// (event-projection driven) — see consumer registrations in cmd/main.go.
type AdminService struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewAdminService(db *gorm.DB, rdb *redis.Client) *AdminService {
	return &AdminService{db: db, rdb: rdb}
}

func (s *AdminService) ctx() context.Context { return context.Background() }

func (s *AdminService) GetUsers(page, size int, search string) ([]model.User, int64, error) {
	var users []model.User
	var total int64
	q := s.db.Model(&model.User{})
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("email ILIKE ? OR full_name ILIKE ?", like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Offset((page - 1) * size).Limit(size).Find(&users).Error
	return users, total, err
}

func (s *AdminService) GetUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AdminService) GetUserKYCDetail(userID uint) (map[string]interface{}, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	var profile model.KYCProfile
	s.db.Where("user_id = ?", userID).First(&profile)
	var docs []model.KYCDocument
	s.db.Where("user_id = ?", userID).Find(&docs)
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
	return s.db.Model(&model.User{}).Where("id = ?", userID).Update("kyc_status", status).Error
}

// GetStats — aggregates platform-wide statistics.
//
// Reads counters maintained in Redis by event consumers (each owning service
// publishes balance.changed / trade.executed; this service projects into
// Redis hash "admin:stats"). Database-per-service forbids cross-DB queries.
func (s *AdminService) GetStats() (map[string]interface{}, error) {
	var totalUsers int64
	if err := s.db.Model(&model.User{}).
		Where("role <> ?", "SYSTEM").
		Count(&totalUsers).Error; err != nil {
		return nil, err
	}

	// Counters populated by Redis event projections.
	stats := map[string]interface{}{
		"totalUsers":              totalUsers,
		"volume24h":               s.statRedisFloat("admin:stats:volume24h"),
		"activeOrders":            s.statRedisInt("admin:stats:active_orders"),
		"totalDeposited":          s.statRedisFloat("admin:stats:total_deposited"),
		"pendingWithdrawalsCount": s.statRedisInt("admin:stats:pending_withdrawals_count"),
		"pendingWithdrawalsSum":   s.statRedisFloat("admin:stats:pending_withdrawals_sum"),
		"activeFuturesPositions":  s.statRedisInt("admin:stats:active_futures"),
	}
	return stats, nil
}

// statRedisFloat returns the named counter; 0 on miss/error (admin dashboard tolerates it).
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
	since := time.Now().AddDate(0, 0, -30)

	type dailyCount struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}

	var userGrowth []dailyCount
	if err := s.db.Model(&model.User{}).
		Select("DATE(created_at) AS date, COUNT(*) AS count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&userGrowth).Error; err != nil {
		return nil, err
	}

	var pendingKyc int64
	s.db.Model(&model.User{}).Where("kyc_status = ?", "PENDING").Count(&pendingKyc)

	// Pending deposits count owned by wallet-service; read from Redis projection.
	pendingDeposits := s.statRedisInt("admin:stats:pending_deposits")

	return map[string]interface{}{
		"userGrowth":      userGrowth,
		"pendingKyc":      pendingKyc,
		"pendingDeposits": pendingDeposits,
	}, nil
}
