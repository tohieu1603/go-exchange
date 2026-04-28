package repository

import (
	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type ReferralRepo interface {
	CreateCode(c *model.ReferralCode) error
	FindCodeByValue(code string) (*model.ReferralCode, error)
	FindDefaultByUser(userID uint) (*model.ReferralCode, error)
	IncrementUsage(codeID uint) error

	CreateReferral(r *model.Referral) error
	FindReferralByReferee(refereeID uint) (*model.Referral, error)
	ListReferees(referrerID uint, page, size int) ([]model.Referral, int64, error)

	CreateCommission(c *model.ReferralCommission) error
	FindCommissionByTrade(tradeID uint) (*model.ReferralCommission, error)
	SumCommissionByUser(referrerID uint) (float64, error)
	ListCommissions(referrerID uint, page, size int) ([]model.ReferralCommission, int64, error)
}

type referralRepo struct{ db *gorm.DB }

func NewReferralRepo(db *gorm.DB) ReferralRepo { return &referralRepo{db: db} }

func (r *referralRepo) CreateCode(c *model.ReferralCode) error { return r.db.Create(c).Error }

func (r *referralRepo) FindCodeByValue(code string) (*model.ReferralCode, error) {
	var c model.ReferralCode
	err := r.db.Where("code = ?", code).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *referralRepo) FindDefaultByUser(userID uint) (*model.ReferralCode, error) {
	var c model.ReferralCode
	err := r.db.Where("user_id = ? AND is_default = true", userID).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *referralRepo) IncrementUsage(codeID uint) error {
	return r.db.Model(&model.ReferralCode{}).Where("id = ?", codeID).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).Error
}

func (r *referralRepo) CreateReferral(rr *model.Referral) error { return r.db.Create(rr).Error }

func (r *referralRepo) FindReferralByReferee(refereeID uint) (*model.Referral, error) {
	var ref model.Referral
	err := r.db.Where("referee_id = ?", refereeID).First(&ref).Error
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (r *referralRepo) ListReferees(referrerID uint, page, size int) ([]model.Referral, int64, error) {
	var rows []model.Referral
	var total int64
	q := r.db.Model(&model.Referral{}).Where("referrer_id = ?", referrerID)
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error
	return rows, total, err
}

func (r *referralRepo) CreateCommission(c *model.ReferralCommission) error {
	// idempotent on trade_id (unique index): ON CONFLICT DO NOTHING
	return r.db.Where("trade_id = ?", c.TradeID).
		FirstOrCreate(c).Error
}

func (r *referralRepo) FindCommissionByTrade(tradeID uint) (*model.ReferralCommission, error) {
	var c model.ReferralCommission
	err := r.db.Where("trade_id = ?", tradeID).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *referralRepo) SumCommissionByUser(referrerID uint) (float64, error) {
	var sum float64
	err := r.db.Model(&model.ReferralCommission{}).
		Where("referrer_id = ?", referrerID).
		Select("COALESCE(SUM(commission), 0)").Scan(&sum).Error
	return sum, err
}

func (r *referralRepo) ListCommissions(referrerID uint, page, size int) ([]model.ReferralCommission, int64, error) {
	var rows []model.ReferralCommission
	var total int64
	q := r.db.Model(&model.ReferralCommission{}).Where("referrer_id = ?", referrerID)
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error
	return rows, total, err
}
