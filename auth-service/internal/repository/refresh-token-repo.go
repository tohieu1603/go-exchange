package repository

import (
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

// RefreshTokenRepo persists refresh tokens with rotation chain (family) tracking.
type RefreshTokenRepo interface {
	Create(rt *model.RefreshToken) error
	FindByHash(tokenHash string) (*model.RefreshToken, error)
	MarkUsed(id uint) error
	RevokeFamily(familyID, reason string) error
	RevokeByUser(userID uint, reason string) error
	RevokeByID(id uint, reason string) error
}

type refreshTokenRepo struct{ db *gorm.DB }

func NewRefreshTokenRepo(db *gorm.DB) RefreshTokenRepo {
	return &refreshTokenRepo{db: db}
}

func (r *refreshTokenRepo) Create(rt *model.RefreshToken) error {
	return r.db.Create(rt).Error
}

func (r *refreshTokenRepo) FindByHash(tokenHash string) (*model.RefreshToken, error) {
	var rt model.RefreshToken
	err := r.db.Where("token_hash = ?", tokenHash).First(&rt).Error
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *refreshTokenRepo) MarkUsed(id uint) error {
	now := time.Now()
	return r.db.Model(&model.RefreshToken{}).Where("id = ?", id).
		Update("used_at", now).Error
}

// RevokeFamily revokes all non-revoked tokens in a family — used when replay
// of an already-rotated token is detected (theft signal).
func (r *refreshTokenRepo) RevokeFamily(familyID, reason string) error {
	now := time.Now()
	return r.db.Model(&model.RefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Updates(map[string]interface{}{
			"revoked_at":     now,
			"revoked_reason": reason,
		}).Error
}

func (r *refreshTokenRepo) RevokeByUser(userID uint, reason string) error {
	now := time.Now()
	return r.db.Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]interface{}{
			"revoked_at":     now,
			"revoked_reason": reason,
		}).Error
}

func (r *refreshTokenRepo) RevokeByID(id uint, reason string) error {
	now := time.Now()
	return r.db.Model(&model.RefreshToken{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"revoked_at":     now,
			"revoked_reason": reason,
		}).Error
}
