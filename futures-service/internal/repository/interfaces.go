package repository

import (
	"github.com/cryptox/futures-service/internal/model"
	"gorm.io/gorm"
)

type PositionRepo interface {
	Create(tx *gorm.DB, pos *model.FuturesPosition) error
	FindOpenByUser(userID uint) ([]model.FuturesPosition, error)
	FindByUserAndStatus(userID uint, status string) ([]model.FuturesPosition, error)
	FindAllOpen() ([]model.FuturesPosition, error)
	FindByIDForUpdate(tx *gorm.DB, id uint, status string) (*model.FuturesPosition, error)
	FindByUserAndIDForUpdate(tx *gorm.DB, userID, id uint, status string) (*model.FuturesPosition, error)
	Save(tx *gorm.DB, pos *model.FuturesPosition) error
	UpdateTPSL(id, userID uint, updates map[string]interface{}) error
	FindByUserAndID(userID, id uint, status string) (*model.FuturesPosition, error)
}

// Note: balance reads/writes go via gRPC to wallet-service. No local WalletRepo.
