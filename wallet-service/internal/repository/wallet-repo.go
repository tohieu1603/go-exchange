package repository

import (
	"github.com/cryptox/wallet-service/internal/model"
	"gorm.io/gorm"
)

type walletRepo struct{ db *gorm.DB }

func NewWalletRepo(db *gorm.DB) WalletRepo { return &walletRepo{db: db} }

func (r *walletRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *walletRepo) FindByUserAndCurrency(userID uint, currency string) (*model.Wallet, error) {
	var w model.Wallet
	err := r.db.Where("user_id = ? AND currency = ?", userID, currency).First(&w).Error
	return &w, err
}

func (r *walletRepo) FindAllByUser(userID uint) ([]model.Wallet, error) {
	var wallets []model.Wallet
	err := r.db.Where("user_id = ?", userID).Order("currency").Find(&wallets).Error
	return wallets, err
}

func (r *walletRepo) UpdateBalance(tx *gorm.DB, userID uint, currency string, delta float64) error {
	if delta < 0 {
		return r.getDB(tx).Exec(`
			UPDATE wallets SET balance = balance + ?, updated_at = NOW()
			WHERE user_id = ? AND currency = ? AND balance >= ?
		`, delta, userID, currency, -delta).Error
	}
	return r.getDB(tx).Exec(`
		UPDATE wallets SET balance = balance + ?, updated_at = NOW()
		WHERE user_id = ? AND currency = ?
	`, delta, userID, currency).Error
}

func (r *walletRepo) LockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error {
	result := r.getDB(tx).Exec(`
		UPDATE wallets SET locked_balance = locked_balance + ?, updated_at = NOW()
		WHERE user_id = ? AND currency = ? AND (balance - locked_balance) >= ?
	`, amount, userID, currency, amount)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *walletRepo) UnlockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error {
	return r.getDB(tx).Exec(`
		UPDATE wallets SET locked_balance = GREATEST(locked_balance - ?, 0), updated_at = NOW()
		WHERE user_id = ? AND currency = ?
	`, amount, userID, currency).Error
}

func (r *walletRepo) CreateBatch(tx *gorm.DB, wallets []model.Wallet) error {
	return r.getDB(tx).CreateInBatches(wallets, 100).Error
}

func (r *walletRepo) Upsert(tx *gorm.DB, userID uint, currency string, balance float64) error {
	result := r.getDB(tx).Exec(`
		UPDATE wallets SET balance = balance + ?, updated_at = NOW()
		WHERE user_id = ? AND currency = ?
	`, balance, userID, currency)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return r.getDB(tx).Create(&model.Wallet{UserID: userID, Currency: currency, Balance: balance}).Error
	}
	return nil
}
