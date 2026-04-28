package repository

import (
	"github.com/cryptox/wallet-service/internal/model"
	"gorm.io/gorm"
)

type depositRepo struct{ db *gorm.DB }

func NewDepositRepo(db *gorm.DB) DepositRepo { return &depositRepo{db: db} }

func (r *depositRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *depositRepo) Create(tx *gorm.DB, d *model.Deposit) error {
	return r.getDB(tx).Create(d).Error
}

func (r *depositRepo) FindByOrderCode(code string) (*model.Deposit, error) {
	var d model.Deposit
	err := r.db.Where("order_code = ?", code).First(&d).Error
	return &d, err
}

func (r *depositRepo) FindByUser(userID uint, page, size int) ([]model.Deposit, int64, error) {
	var deposits []model.Deposit
	var total int64
	offset := (page - 1) * size

	r.db.Model(&model.Deposit{}).Where("user_id = ?", userID).Count(&total)
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").
		Limit(size).Offset(offset).Find(&deposits).Error
	return deposits, total, err
}

func (r *depositRepo) Save(tx *gorm.DB, d *model.Deposit) error {
	return r.getDB(tx).Save(d).Error
}
