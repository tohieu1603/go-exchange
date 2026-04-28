package repository

import (
	"time"

	"github.com/cryptox/futures-service/internal/model"
	"gorm.io/gorm"
)

type FundingRepo interface {
	CreateRate(r *model.FundingRate) error
	LatestRate(pair string) (*model.FundingRate, error)
	RecentRates(pair string, limit int) ([]model.FundingRate, error)

	CreatePayment(p *model.FundingPayment) error
	HistoryByUser(userID uint, page, size int) ([]model.FundingPayment, int64, error)
}

type fundingRepo struct{ db *gorm.DB }

func NewFundingRepo(db *gorm.DB) FundingRepo { return &fundingRepo{db: db} }

func (r *fundingRepo) CreateRate(rate *model.FundingRate) error {
	return r.db.Where("pair = ? AND settled_at = ?", rate.Pair, rate.SettledAt).
		FirstOrCreate(rate).Error
}

func (r *fundingRepo) LatestRate(pair string) (*model.FundingRate, error) {
	var rate model.FundingRate
	err := r.db.Where("pair = ?", pair).Order("settled_at DESC").First(&rate).Error
	if err != nil {
		return nil, err
	}
	return &rate, nil
}

func (r *fundingRepo) RecentRates(pair string, limit int) ([]model.FundingRate, error) {
	var rows []model.FundingRate
	err := r.db.Where("pair = ?", pair).Order("settled_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *fundingRepo) CreatePayment(p *model.FundingPayment) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	// Idempotent on (position_id, funding_rate_id) via unique index.
	return r.db.Where("position_id = ? AND funding_rate_id = ?", p.PositionID, p.FundingRateID).
		FirstOrCreate(p).Error
}

func (r *fundingRepo) HistoryByUser(userID uint, page, size int) ([]model.FundingPayment, int64, error) {
	var rows []model.FundingPayment
	var total int64
	q := r.db.Model(&model.FundingPayment{}).Where("user_id = ?", userID)
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error
	return rows, total, err
}
