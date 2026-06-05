package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BonusRepo is the pgx+sqlc adapter for usecase.BonusRepo.
type BonusRepo struct{ q *sqlc.Queries }

func NewBonusRepo(pool *pgxpool.Pool) *BonusRepo { return &BonusRepo{q: sqlc.New(pool)} }

var _ usecase.BonusRepo = (*BonusRepo)(nil)

func (r *BonusRepo) CreatePromotion(ctx context.Context, p *domain.BonusPromotion) error {
	row, err := r.q.CreatePromotion(ctx, sqlc.CreatePromotionParams{
		Name: p.Name, Description: p.Description, BonusPercent: p.BonusPercent, MaxBonusAmount: p.MaxBonusAmount,
		TargetType: p.TargetType, TargetUserIds: p.TargetUserIDs, TriggerType: p.TriggerType,
		MinDeposit: p.MinDeposit, IsActive: p.IsActive, StartAt: p.StartAt, EndAt: p.EndAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create promotion: %w", err)
	}
	p.ID = uint(row.ID)
	p.CreatedAt = row.CreatedAt
	return nil
}

func (r *BonusRepo) FindActivePromotions(ctx context.Context) ([]domain.BonusPromotion, error) {
	rows, err := r.q.FindActivePromotions(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: find active promotions: %w", err)
	}
	return promosToDomain(rows), nil
}

func (r *BonusRepo) FindAllPromotions(ctx context.Context) ([]domain.BonusPromotion, error) {
	rows, err := r.q.FindAllPromotions(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: find all promotions: %w", err)
	}
	return promosToDomain(rows), nil
}

func (r *BonusRepo) FindPromotionByID(ctx context.Context, id uint) (*domain.BonusPromotion, error) {
	row, err := r.q.FindPromotionByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: find promotion: %w", err)
	}
	p := promoToDomain(row)
	return &p, nil
}

func (r *BonusRepo) UpdatePromotion(ctx context.Context, p *domain.BonusPromotion) error {
	return wrap("update promotion", r.q.UpdatePromotion(ctx, sqlc.UpdatePromotionParams{
		ID: int64(p.ID), Name: p.Name, Description: p.Description, BonusPercent: p.BonusPercent,
		MaxBonusAmount: p.MaxBonusAmount, TargetType: p.TargetType, TargetUserIds: p.TargetUserIDs,
		TriggerType: p.TriggerType, MinDeposit: p.MinDeposit, IsActive: p.IsActive, StartAt: p.StartAt, EndAt: p.EndAt,
	}))
}

func (r *BonusRepo) CreateUserBonus(ctx context.Context, b *domain.UserBonus) error {
	row, err := r.q.CreateUserBonus(ctx, sqlc.CreateUserBonusParams{
		UserID: int64(b.UserID), PromotionID: int64(b.PromotionID), DepositID: int64(b.DepositID),
		BonusAmount: b.BonusAmount, UsedAmount: b.UsedAmount, Status: defaultStr(b.Status, "ACTIVE"), ExpiresAt: b.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create user bonus: %w", err)
	}
	b.ID = uint(row.ID)
	b.CreatedAt = row.CreatedAt
	return nil
}

func (r *BonusRepo) FindUserBonuses(ctx context.Context, userID uint) ([]domain.UserBonus, error) {
	rows, err := r.q.FindUserBonuses(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: find user bonuses: %w", err)
	}
	return userBonusesToDomain(rows), nil
}

func (r *BonusRepo) FindActiveUserBonuses(ctx context.Context, userID uint) ([]domain.UserBonus, error) {
	rows, err := r.q.FindActiveUserBonuses(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: find active user bonuses: %w", err)
	}
	return userBonusesToDomain(rows), nil
}

func (r *BonusRepo) UpdateUserBonus(ctx context.Context, b *domain.UserBonus) error {
	return wrap("update user bonus", r.q.UpdateUserBonus(ctx, sqlc.UpdateUserBonusParams{
		ID: int64(b.ID), UserID: int64(b.UserID), PromotionID: int64(b.PromotionID), DepositID: int64(b.DepositID),
		BonusAmount: b.BonusAmount, UsedAmount: b.UsedAmount, Status: b.Status, ExpiresAt: b.ExpiresAt,
	}))
}

func (r *BonusRepo) SumActiveBonus(ctx context.Context, userID uint) (float64, error) {
	return r.q.SumActiveBonus(ctx, int64(userID))
}

func promoToDomain(m sqlc.BonusPromotion) domain.BonusPromotion {
	return domain.BonusPromotion{
		ID: uint(m.ID), Name: m.Name, Description: m.Description, BonusPercent: m.BonusPercent,
		MaxBonusAmount: m.MaxBonusAmount, TargetType: m.TargetType, TargetUserIDs: m.TargetUserIds,
		TriggerType: m.TriggerType, MinDeposit: m.MinDeposit, IsActive: m.IsActive,
		StartAt: m.StartAt, EndAt: m.EndAt, CreatedAt: m.CreatedAt,
	}
}

func promosToDomain(ms []sqlc.BonusPromotion) []domain.BonusPromotion {
	out := make([]domain.BonusPromotion, len(ms))
	for i, m := range ms {
		out[i] = promoToDomain(m)
	}
	return out
}

func userBonusToDomain(m sqlc.UserBonu) domain.UserBonus {
	return domain.UserBonus{
		ID: uint(m.ID), UserID: uint(m.UserID), PromotionID: uint(m.PromotionID), DepositID: uint(m.DepositID),
		BonusAmount: m.BonusAmount, UsedAmount: m.UsedAmount, Status: m.Status, CreatedAt: m.CreatedAt, ExpiresAt: m.ExpiresAt,
	}
}

func userBonusesToDomain(ms []sqlc.UserBonu) []domain.UserBonus {
	out := make([]domain.UserBonus, len(ms))
	for i, m := range ms {
		out[i] = userBonusToDomain(m)
	}
	return out
}
