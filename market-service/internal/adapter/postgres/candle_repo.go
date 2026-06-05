// Package postgres implements domain.CandleRepository on top of pgx + sqlc.
package postgres

import (
	"context"
	"fmt"

	"github.com/cryptox/market-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/market-service/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CandleRepo is the pgx+sqlc adapter for domain.CandleRepository.
type CandleRepo struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewCandleRepo(pool *pgxpool.Pool) *CandleRepo { return &CandleRepo{pool: pool, q: sqlc.New(pool)} }

var _ domain.CandleRepository = (*CandleRepo)(nil)

func (r *CandleRepo) Upsert(ctx context.Context, c domain.Candle) error {
	if err := r.q.UpsertCandle(ctx, toParams(c)); err != nil {
		return fmt.Errorf("postgres: upsert candle: %w", err)
	}
	return nil
}

// UpsertBatch writes many candles in a single transaction (historical backfill).
func (r *CandleRepo) UpsertBatch(ctx context.Context, candles []domain.Candle) error {
	if len(candles) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin batch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)
	for _, c := range candles {
		if err := qtx.UpsertCandle(ctx, toParams(c)); err != nil {
			return fmt.Errorf("postgres: upsert candle batch: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit batch: %w", err)
	}
	return nil
}

func (r *CandleRepo) Query(ctx context.Context, pair, interval string, limit int) ([]domain.Candle, error) {
	rows, err := r.q.QueryCandles(ctx, sqlc.QueryCandlesParams{Pair: pair, Interval: interval, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("postgres: query candles: %w", err)
	}
	out := make([]domain.Candle, len(rows))
	for i, m := range rows {
		out[i] = domain.Candle{
			Pair: m.Pair, Interval: m.Interval, OpenTime: m.OpenTime,
			Open: m.Open, High: m.High, Low: m.Low, Close: m.Close, Volume: m.Volume,
		}
	}
	return out, nil
}

func toParams(c domain.Candle) sqlc.UpsertCandleParams {
	return sqlc.UpsertCandleParams{
		Pair: c.Pair, Interval: c.Interval, OpenTime: c.OpenTime,
		Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
	}
}
