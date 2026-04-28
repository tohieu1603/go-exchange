package repository

import (
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type APIKeyRepo interface {
	Create(k *model.APIKey) error
	FindByKeyID(keyID string) (*model.APIKey, error)
	ListByUser(userID uint) ([]model.APIKey, error)
	Revoke(id, userID uint) error
	UpdateLastUsed(id uint, ip string) error
}

type apiKeyRepo struct{ db *gorm.DB }

func NewAPIKeyRepo(db *gorm.DB) APIKeyRepo { return &apiKeyRepo{db: db} }

func (r *apiKeyRepo) Create(k *model.APIKey) error { return r.db.Create(k).Error }

func (r *apiKeyRepo) FindByKeyID(keyID string) (*model.APIKey, error) {
	var k model.APIKey
	err := r.db.Where("key_id = ? AND revoked_at IS NULL", keyID).First(&k).Error
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *apiKeyRepo) ListByUser(userID uint) ([]model.APIKey, error) {
	var keys []model.APIKey
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func (r *apiKeyRepo) Revoke(id, userID uint) error {
	now := time.Now()
	return r.db.Model(&model.APIKey{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("revoked_at", now).Error
}

func (r *apiKeyRepo) UpdateLastUsed(id uint, ip string) error {
	now := time.Now()
	return r.db.Model(&model.APIKey{}).Where("id = ?", id).
		Updates(map[string]interface{}{"last_used_at": now, "last_used_ip": ip}).Error
}
