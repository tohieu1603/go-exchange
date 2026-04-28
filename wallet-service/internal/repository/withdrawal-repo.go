package repository

import (
	"github.com/cryptox/wallet-service/internal/model"
	"gorm.io/gorm"
)

type withdrawalRepo struct{ db *gorm.DB }

func NewWithdrawalRepo(db *gorm.DB) WithdrawalRepo { return &withdrawalRepo{db: db} }

func (r *withdrawalRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *withdrawalRepo) Create(tx *gorm.DB, w *model.Withdrawal) error {
	return r.getDB(tx).Create(w).Error
}

func (r *withdrawalRepo) FindByID(id uint) (*model.Withdrawal, error) {
	var w model.Withdrawal
	err := r.db.First(&w, id).Error
	return &w, err
}

func (r *withdrawalRepo) FindByUser(userID uint, page, size int) ([]model.Withdrawal, int64, error) {
	var withdrawals []model.Withdrawal
	var total int64
	offset := (page - 1) * size

	r.db.Model(&model.Withdrawal{}).Where("user_id = ?", userID).Count(&total)
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").
		Limit(size).Offset(offset).Find(&withdrawals).Error
	return withdrawals, total, err
}

func (r *withdrawalRepo) FindPending(page, size int) ([]model.Withdrawal, int64, error) {
	var withdrawals []model.Withdrawal
	var total int64
	offset := (page - 1) * size

	r.db.Model(&model.Withdrawal{}).Where("status = ?", "PENDING").Count(&total)
	err := r.db.Where("status = ?", "PENDING").Order("created_at DESC").
		Limit(size).Offset(offset).Find(&withdrawals).Error
	return withdrawals, total, err
}

func (r *withdrawalRepo) Save(tx *gorm.DB, w *model.Withdrawal) error {
	return r.getDB(tx).Save(w).Error
}

func (r *withdrawalRepo) FindLatestPendingByUser(userID uint) (*model.Withdrawal, error) {
	var w model.Withdrawal
	err := r.db.Where("user_id = ? AND status = ?", userID, "PENDING").
		Order("created_at DESC").First(&w).Error
	return &w, err
}
