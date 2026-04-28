package repository

import (
	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type BonusRepo interface {
	CreatePromotion(tx *gorm.DB, promo *model.BonusPromotion) error
	FindActivePromotions() ([]model.BonusPromotion, error)
	FindAllPromotions() ([]model.BonusPromotion, error)
	FindPromotionByID(id uint) (*model.BonusPromotion, error)
	UpdatePromotion(tx *gorm.DB, promo *model.BonusPromotion) error
	CreateUserBonus(tx *gorm.DB, bonus *model.UserBonus) error
	FindUserBonuses(userID uint) ([]model.UserBonus, error)
	FindActiveUserBonuses(userID uint) ([]model.UserBonus, error)
	UpdateUserBonus(tx *gorm.DB, bonus *model.UserBonus) error
	SumActiveBonus(userID uint) (float64, error)
}

type gormBonusRepo struct{ db *gorm.DB }

func NewBonusRepo(db *gorm.DB) BonusRepo { return &gormBonusRepo{db: db} }

func (r *gormBonusRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *gormBonusRepo) CreatePromotion(tx *gorm.DB, promo *model.BonusPromotion) error {
	return r.getDB(tx).Create(promo).Error
}

func (r *gormBonusRepo) FindActivePromotions() ([]model.BonusPromotion, error) {
	var promos []model.BonusPromotion
	err := r.db.Where("is_active = ?", true).Find(&promos).Error
	return promos, err
}

func (r *gormBonusRepo) FindAllPromotions() ([]model.BonusPromotion, error) {
	var promos []model.BonusPromotion
	err := r.db.Order("created_at DESC").Find(&promos).Error
	return promos, err
}

func (r *gormBonusRepo) FindPromotionByID(id uint) (*model.BonusPromotion, error) {
	var promo model.BonusPromotion
	err := r.db.First(&promo, id).Error
	return &promo, err
}

func (r *gormBonusRepo) UpdatePromotion(tx *gorm.DB, promo *model.BonusPromotion) error {
	return r.getDB(tx).Save(promo).Error
}

func (r *gormBonusRepo) CreateUserBonus(tx *gorm.DB, bonus *model.UserBonus) error {
	return r.getDB(tx).Create(bonus).Error
}

func (r *gormBonusRepo) FindUserBonuses(userID uint) ([]model.UserBonus, error) {
	var bonuses []model.UserBonus
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&bonuses).Error
	return bonuses, err
}

func (r *gormBonusRepo) FindActiveUserBonuses(userID uint) ([]model.UserBonus, error) {
	var bonuses []model.UserBonus
	err := r.db.Where("user_id = ? AND status = ?", userID, "ACTIVE").
		Order("created_at ASC").Find(&bonuses).Error
	return bonuses, err
}

func (r *gormBonusRepo) UpdateUserBonus(tx *gorm.DB, bonus *model.UserBonus) error {
	return r.getDB(tx).Save(bonus).Error
}

func (r *gormBonusRepo) SumActiveBonus(userID uint) (float64, error) {
	var total float64
	err := r.db.Model(&model.UserBonus{}).
		Where("user_id = ? AND status = ?", userID, "ACTIVE").
		Select("COALESCE(SUM(bonus_amount - used_amount), 0)").
		Scan(&total).Error
	return total, err
}
