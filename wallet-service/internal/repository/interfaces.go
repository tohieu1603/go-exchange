package repository

import (
	"github.com/cryptox/wallet-service/internal/model"
	"gorm.io/gorm"
)

type WalletRepo interface {
	FindByUserAndCurrency(userID uint, currency string) (*model.Wallet, error)
	FindAllByUser(userID uint) ([]model.Wallet, error)
	UpdateBalance(tx *gorm.DB, userID uint, currency string, delta float64) error
	LockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error
	UnlockBalance(tx *gorm.DB, userID uint, currency string, amount float64) error
	CreateBatch(tx *gorm.DB, wallets []model.Wallet) error
	Upsert(tx *gorm.DB, userID uint, currency string, balance float64) error
}

type DepositRepo interface {
	Create(tx *gorm.DB, d *model.Deposit) error
	FindByOrderCode(code string) (*model.Deposit, error)
	FindByUser(userID uint, page, size int) ([]model.Deposit, int64, error)
	Save(tx *gorm.DB, d *model.Deposit) error
}

type WithdrawalRepo interface {
	Create(tx *gorm.DB, w *model.Withdrawal) error
	FindByID(id uint) (*model.Withdrawal, error)
	FindByUser(userID uint, page, size int) ([]model.Withdrawal, int64, error)
	FindPending(page, size int) ([]model.Withdrawal, int64, error)
	Save(tx *gorm.DB, w *model.Withdrawal) error
	FindLatestPendingByUser(userID uint) (*model.Withdrawal, error)
}
