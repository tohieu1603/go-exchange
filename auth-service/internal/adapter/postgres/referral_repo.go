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

// ReferralRepo is the pgx+sqlc adapter for usecase.ReferralRepo.
type ReferralRepo struct{ q *sqlc.Queries }

func NewReferralRepo(pool *pgxpool.Pool) *ReferralRepo { return &ReferralRepo{q: sqlc.New(pool)} }

var _ usecase.ReferralRepo = (*ReferralRepo)(nil)

func (r *ReferralRepo) CreateCode(ctx context.Context, c *domain.ReferralCode) error {
	row, err := r.q.CreateReferralCode(ctx, sqlc.CreateReferralCodeParams{
		UserID: int64(c.UserID), Code: c.Code, IsDefault: c.IsDefault, UsageCount: int32(c.UsageCount),
	})
	if err != nil {
		return fmt.Errorf("postgres: create referral code: %w", err)
	}
	c.ID = uint(row.ID)
	c.CreatedAt = row.CreatedAt
	return nil
}

func (r *ReferralRepo) FindCodeByValue(ctx context.Context, code string) (*domain.ReferralCode, error) {
	row, err := r.q.GetReferralCodeByValue(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get referral code: %w", err)
	}
	c := codeToDomain(row)
	return &c, nil
}

func (r *ReferralRepo) FindDefaultByUser(ctx context.Context, userID uint) (*domain.ReferralCode, error) {
	row, err := r.q.GetDefaultReferralCode(ctx, int64(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get default referral code: %w", err)
	}
	c := codeToDomain(row)
	return &c, nil
}

func (r *ReferralRepo) IncrementUsage(ctx context.Context, codeID uint) error {
	return wrap("increment referral usage", r.q.IncrementReferralUsage(ctx, int64(codeID)))
}

func (r *ReferralRepo) CreateReferral(ctx context.Context, rr *domain.Referral) error {
	row, err := r.q.CreateReferral(ctx, sqlc.CreateReferralParams{
		ReferrerID: int64(rr.ReferrerID), RefereeID: int64(rr.RefereeID), Code: rr.Code, Tier: int32(rr.Tier),
	})
	if err != nil {
		return fmt.Errorf("postgres: create referral: %w", err)
	}
	rr.ID = uint(row.ID)
	rr.CreatedAt = row.CreatedAt
	return nil
}

func (r *ReferralRepo) FindReferralByReferee(ctx context.Context, refereeID uint) (*domain.Referral, error) {
	row, err := r.q.GetReferralByReferee(ctx, int64(refereeID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get referral by referee: %w", err)
	}
	ref := referralToDomain(row)
	return &ref, nil
}

func (r *ReferralRepo) ListReferees(ctx context.Context, referrerID uint, page, size int) ([]domain.Referral, int64, error) {
	rows, err := r.q.ListReferees(ctx, sqlc.ListRefereesParams{ReferrerID: int64(referrerID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list referees: %w", err)
	}
	total, err := r.q.CountReferees(ctx, int64(referrerID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count referees: %w", err)
	}
	out := make([]domain.Referral, len(rows))
	for i, m := range rows {
		out[i] = referralToDomain(m)
	}
	return out, total, nil
}

func (r *ReferralRepo) CreateCommission(ctx context.Context, c *domain.ReferralCommission) error {
	row, err := r.q.CreateReferralCommission(ctx, sqlc.CreateReferralCommissionParams{
		ReferrerID: int64(c.ReferrerID), RefereeID: int64(c.RefereeID), TradeID: int64(c.TradeID),
		Currency: c.Currency, FeeAmount: c.FeeAmount, Rate: c.Rate, Commission: c.Commission,
	})
	if err != nil {
		return fmt.Errorf("postgres: create referral commission: %w", err)
	}
	c.ID = uint(row.ID)
	c.CreatedAt = row.CreatedAt
	return nil
}

func (r *ReferralRepo) FindCommissionByTrade(ctx context.Context, tradeID uint) (*domain.ReferralCommission, error) {
	row, err := r.q.GetReferralCommissionByTrade(ctx, int64(tradeID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get commission by trade: %w", err)
	}
	c := commissionToDomain(row)
	return &c, nil
}

func (r *ReferralRepo) SumCommissionByUser(ctx context.Context, referrerID uint) (float64, error) {
	return r.q.SumCommissionByUser(ctx, int64(referrerID))
}

func (r *ReferralRepo) ListCommissions(ctx context.Context, referrerID uint, page, size int) ([]domain.ReferralCommission, int64, error) {
	rows, err := r.q.ListCommissions(ctx, sqlc.ListCommissionsParams{ReferrerID: int64(referrerID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list commissions: %w", err)
	}
	total, err := r.q.CountCommissions(ctx, int64(referrerID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count commissions: %w", err)
	}
	out := make([]domain.ReferralCommission, len(rows))
	for i, m := range rows {
		out[i] = commissionToDomain(m)
	}
	return out, total, nil
}

func codeToDomain(m sqlc.ReferralCode) domain.ReferralCode {
	return domain.ReferralCode{ID: uint(m.ID), UserID: uint(m.UserID), Code: m.Code, IsDefault: m.IsDefault, UsageCount: int(m.UsageCount), CreatedAt: m.CreatedAt}
}

func referralToDomain(m sqlc.Referral) domain.Referral {
	return domain.Referral{ID: uint(m.ID), ReferrerID: uint(m.ReferrerID), RefereeID: uint(m.RefereeID), Code: m.Code, Tier: int(m.Tier), CreatedAt: m.CreatedAt}
}

func commissionToDomain(m sqlc.ReferralCommission) domain.ReferralCommission {
	return domain.ReferralCommission{
		ID: uint(m.ID), ReferrerID: uint(m.ReferrerID), RefereeID: uint(m.RefereeID), TradeID: uint(m.TradeID),
		Currency: m.Currency, FeeAmount: m.FeeAmount, Rate: m.Rate, Commission: m.Commission, CreatedAt: m.CreatedAt,
	}
}
