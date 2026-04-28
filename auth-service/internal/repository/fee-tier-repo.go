package repository

import (
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type FeeTierRepo interface {
	ListAll() ([]model.FeeTier, error)
	GetByLevel(level int) (*model.FeeTier, error)
	GetUserVolume(userID uint) (*model.UserVolume30d, error)
	UpsertVolume(userID uint, volume float64, level int) error
	IncrementVolume(userID uint, delta float64) error
	SeedDefaults() error
}

type feeTierRepo struct{ db *gorm.DB }

func NewFeeTierRepo(db *gorm.DB) FeeTierRepo { return &feeTierRepo{db: db} }

func (r *feeTierRepo) ListAll() ([]model.FeeTier, error) {
	var tiers []model.FeeTier
	err := r.db.Order("level ASC").Find(&tiers).Error
	return tiers, err
}

func (r *feeTierRepo) GetByLevel(level int) (*model.FeeTier, error) {
	var t model.FeeTier
	err := r.db.Where("level = ?", level).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *feeTierRepo) GetUserVolume(userID uint) (*model.UserVolume30d, error) {
	var v model.UserVolume30d
	err := r.db.Where("user_id = ?", userID).First(&v).Error
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *feeTierRepo) UpsertVolume(userID uint, volume float64, level int) error {
	v := model.UserVolume30d{UserID: userID, Volume: volume, TierLevel: level, UpdatedAt: time.Now()}
	return r.db.Save(&v).Error
}

// IncrementVolume atomically adds `delta` to a user's 30-day volume.
// Used by the trade.executed consumer. UPSERT-with-add semantics.
func (r *feeTierRepo) IncrementVolume(userID uint, delta float64) error {
	return r.db.Exec(`
		INSERT INTO user_volume30ds (user_id, volume, tier_level, updated_at)
		VALUES (?, ?, 0, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET volume = user_volume30ds.volume + EXCLUDED.volume,
		    updated_at = NOW()
	`, userID, delta).Error
}

func (r *feeTierRepo) SeedDefaults() error {
	var count int64
	r.db.Model(&model.FeeTier{}).Count(&count)
	if count > 0 {
		return nil
	}
	return r.db.CreateInBatches(model.DefaultFeeTiers, 10).Error
}
