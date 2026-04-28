package repository

import (
	"github.com/cryptox/futures-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type positionRepo struct{ db *gorm.DB }

func NewPositionRepo(db *gorm.DB) PositionRepo { return &positionRepo{db: db} }

func (r *positionRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *positionRepo) Create(tx *gorm.DB, pos *model.FuturesPosition) error {
	return r.getDB(tx).Create(pos).Error
}

func (r *positionRepo) FindOpenByUser(userID uint) ([]model.FuturesPosition, error) {
	var positions []model.FuturesPosition
	err := r.db.Where("user_id = ? AND status = ?", userID, "OPEN").
		Order("created_at DESC").Find(&positions).Error
	return positions, err
}

func (r *positionRepo) FindByUserAndStatus(userID uint, status string) ([]model.FuturesPosition, error) {
	var positions []model.FuturesPosition
	q := r.db.Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("created_at DESC").Find(&positions).Error
	return positions, err
}

func (r *positionRepo) FindAllOpen() ([]model.FuturesPosition, error) {
	var positions []model.FuturesPosition
	err := r.db.Select("id, pair, side, leverage, entry_price, size, liquidation_price, take_profit, stop_loss, user_id, margin").
		Where("status = ?", "OPEN").Find(&positions).Error
	return positions, err
}

func (r *positionRepo) FindByIDForUpdate(tx *gorm.DB, id uint, status string) (*model.FuturesPosition, error) {
	var pos model.FuturesPosition
	err := r.getDB(tx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", id, status).First(&pos).Error
	return &pos, err
}

func (r *positionRepo) FindByUserAndIDForUpdate(tx *gorm.DB, userID, id uint, status string) (*model.FuturesPosition, error) {
	var pos model.FuturesPosition
	err := r.getDB(tx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, status).First(&pos).Error
	return &pos, err
}

func (r *positionRepo) Save(tx *gorm.DB, pos *model.FuturesPosition) error {
	return r.getDB(tx).Save(pos).Error
}

func (r *positionRepo) UpdateTPSL(id, userID uint, updates map[string]interface{}) error {
	return r.db.Model(&model.FuturesPosition{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, "OPEN").
		Updates(updates).Error
}

func (r *positionRepo) FindByUserAndID(userID, id uint, status string) (*model.FuturesPosition, error) {
	var pos model.FuturesPosition
	err := r.db.Where("id = ? AND user_id = ? AND status = ?", id, userID, status).First(&pos).Error
	return &pos, err
}
