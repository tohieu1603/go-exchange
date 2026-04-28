package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cryptox/shared/types"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const platformSettingsKey = "platform_settings"

// SettingsService manages platform-wide configuration (single-row table).
// Reads are served from Redis cache; writes update both DB and cache.
type SettingsService struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewSettingsService(db *gorm.DB, rdb *redis.Client) *SettingsService {
	svc := &SettingsService{db: db, rdb: rdb}
	svc.warmCache()
	return svc
}

// warmCache loads settings from DB into Redis on startup (best-effort).
func (s *SettingsService) warmCache() {
	settings, err := s.getFromDB()
	if err != nil {
		log.Printf("[settings] warm cache error: %v", err)
		return
	}
	s.writeCache(settings)
}

// Get returns platform settings. Tries Redis first, falls back to DB.
func (s *SettingsService) Get() (*types.PlatformSettings, error) {
	ctx := context.Background()
	raw, err := s.rdb.Get(ctx, platformSettingsKey).Bytes()
	if err == nil {
		var settings types.PlatformSettings
		if jsonErr := json.Unmarshal(raw, &settings); jsonErr == nil {
			return &settings, nil
		}
	}
	// Cache miss or decode error: read DB and repopulate
	settings, err := s.getFromDB()
	if err != nil {
		return nil, err
	}
	s.writeCache(settings)
	return settings, nil
}

// Update validates and persists platform settings, then refreshes cache.
func (s *SettingsService) Update(settings *types.PlatformSettings) error {
	if err := validateSettings(settings); err != nil {
		return err
	}
	settings.ID = 1
	if err := s.db.Save(settings).Error; err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	s.writeCache(settings)
	return nil
}

func (s *SettingsService) getFromDB() (*types.PlatformSettings, error) {
	var settings types.PlatformSettings
	err := s.db.First(&settings, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		defaults := types.DefaultPlatformSettings()
		if createErr := s.db.Create(&defaults).Error; createErr != nil {
			return nil, fmt.Errorf("create default settings: %w", createErr)
		}
		return &defaults, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch settings: %w", err)
	}
	return &settings, nil
}

func (s *SettingsService) writeCache(settings *types.PlatformSettings) {
	data, err := json.Marshal(settings)
	if err != nil {
		return
	}
	s.rdb.Set(context.Background(), platformSettingsKey, data, 24*time.Hour)
}

func validateSettings(s *types.PlatformSettings) error {
	if s.DepositFeePercent < 0 || s.DepositFeePercent > 100 {
		return errors.New("depositFeePercent must be between 0 and 100")
	}
	if s.WithdrawFeePercent < 0 || s.WithdrawFeePercent > 100 {
		return errors.New("withdrawFeePercent must be between 0 and 100")
	}
	if s.TradingFeePercent < 0 || s.TradingFeePercent > 100 {
		return errors.New("tradingFeePercent must be between 0 and 100")
	}
	if s.MinDeposit < 0 {
		return errors.New("minDeposit must be non-negative")
	}
	if s.MaxDeposit > 0 && s.MinDeposit >= s.MaxDeposit {
		return errors.New("minDeposit must be less than maxDeposit")
	}
	if s.MinWithdraw < 0 {
		return errors.New("minWithdraw must be non-negative")
	}
	if s.MaxWithdraw > 0 && s.MinWithdraw >= s.MaxWithdraw {
		return errors.New("minWithdraw must be less than maxWithdraw")
	}
	return nil
}
