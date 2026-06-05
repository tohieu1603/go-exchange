package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/futures-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/futures-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FundingRepo is the pgx+sqlc adapter for domain.FundingRepository.
type FundingRepo struct{ pool *pgxpool.Pool }

func NewFundingRepo(pool *pgxpool.Pool) *FundingRepo { return &FundingRepo{pool: pool} }

var _ domain.FundingRepository = (*FundingRepo)(nil)

// CreateRate is idempotent on (pair, settled_at); the conflict path returns the
// existing row's id.
func (r *FundingRepo) CreateRate(ctx context.Context, fr *domain.FundingRate) error {
	interval := fr.Interval
	if interval == "" {
		interval = "8h"
	}
	id, err := q(ctx, r.pool).CreateFundingRate(ctx, sqlc.CreateFundingRateParams{
		Pair: fr.Pair, Rate: fr.Rate, IndexPrice: fr.IndexPrice, MarkPrice: fr.MarkPrice,
		Interval: interval, SettledAt: fr.SettledAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create funding rate: %w", err)
	}
	fr.ID = uint(id)
	return nil
}

func (r *FundingRepo) LatestRate(ctx context.Context, pair string) (*domain.FundingRate, error) {
	row, err := q(ctx, r.pool).LatestFundingRate(ctx, pair)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("no funding rate yet")
		}
		return nil, fmt.Errorf("postgres: latest funding rate: %w", err)
	}
	d := fundingRateToDomain(row)
	return &d, nil
}

func (r *FundingRepo) RecentRates(ctx context.Context, pair string, limit int) ([]domain.FundingRate, error) {
	rows, err := q(ctx, r.pool).RecentFundingRates(ctx, sqlc.RecentFundingRatesParams{Pair: pair, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("postgres: recent funding rates: %w", err)
	}
	out := make([]domain.FundingRate, len(rows))
	for i, m := range rows {
		out[i] = fundingRateToDomain(m)
	}
	return out, nil
}

// CreatePayment is idempotent on (position_id, funding_rate_id).
func (r *FundingRepo) CreatePayment(ctx context.Context, p *domain.FundingPayment) error {
	row, err := q(ctx, r.pool).CreateFundingPayment(ctx, sqlc.CreateFundingPaymentParams{
		PositionID: int64(p.PositionID), UserID: int64(p.UserID), FundingRateID: int64(p.FundingRateID),
		Pair: p.Pair, Side: p.Side, Notional: p.Notional, Rate: p.Rate, Amount: p.Amount,
	})
	if err != nil {
		return fmt.Errorf("postgres: create funding payment: %w", err)
	}
	p.ID = uint(row.ID)
	p.CreatedAt = row.CreatedAt
	return nil
}

func (r *FundingRepo) HistoryByUser(ctx context.Context, userID uint, page, size int) ([]domain.FundingPayment, int64, error) {
	qq := q(ctx, r.pool)
	rows, err := qq.FundingHistoryByUser(ctx, sqlc.FundingHistoryByUserParams{UserID: int64(userID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: funding history: %w", err)
	}
	total, err := qq.CountFundingByUser(ctx, int64(userID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count funding: %w", err)
	}
	out := make([]domain.FundingPayment, len(rows))
	for i, m := range rows {
		out[i] = fundingPaymentToDomain(m)
	}
	return out, total, nil
}

func fundingRateToDomain(m sqlc.FundingRate) domain.FundingRate {
	return domain.FundingRate{
		ID: uint(m.ID), Pair: m.Pair, Rate: m.Rate, IndexPrice: m.IndexPrice, MarkPrice: m.MarkPrice,
		Interval: m.Interval, SettledAt: m.SettledAt, CreatedAt: m.CreatedAt,
	}
}

func fundingPaymentToDomain(m sqlc.FundingPayment) domain.FundingPayment {
	return domain.FundingPayment{
		ID: uint(m.ID), PositionID: uint(m.PositionID), UserID: uint(m.UserID), FundingRateID: uint(m.FundingRateID),
		Pair: m.Pair, Side: m.Side, Notional: m.Notional, Rate: m.Rate, Amount: m.Amount, CreatedAt: m.CreatedAt,
	}
}
