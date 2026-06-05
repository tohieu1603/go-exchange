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

// FeeTierRepo is the pgx+sqlc adapter for usecase.FeeTierRepo.
type FeeTierRepo struct{ q *sqlc.Queries }

func NewFeeTierRepo(pool *pgxpool.Pool) *FeeTierRepo { return &FeeTierRepo{q: sqlc.New(pool)} }

var _ usecase.FeeTierRepo = (*FeeTierRepo)(nil)

func (r *FeeTierRepo) ListAll(ctx context.Context) ([]domain.FeeTier, error) {
	rows, err := r.q.ListFeeTiers(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list fee tiers: %w", err)
	}
	out := make([]domain.FeeTier, len(rows))
	for i, m := range rows {
		out[i] = feeTierToDomain(m)
	}
	return out, nil
}

func (r *FeeTierRepo) GetByLevel(ctx context.Context, level int) (*domain.FeeTier, error) {
	row, err := r.q.GetFeeTierByLevel(ctx, int32(level))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get fee tier: %w", err)
	}
	t := feeTierToDomain(row)
	return &t, nil
}

func (r *FeeTierRepo) GetUserVolume(ctx context.Context, userID uint) (*domain.UserVolume30d, error) {
	row, err := r.q.GetUserVolume(ctx, int64(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get user volume: %w", err)
	}
	return &domain.UserVolume30d{UserID: uint(row.UserID), Volume: row.Volume, TierLevel: int(row.TierLevel), UpdatedAt: row.UpdatedAt}, nil
}

func (r *FeeTierRepo) UpsertVolume(ctx context.Context, userID uint, volume float64, level int) error {
	return wrap("upsert volume", r.q.UpsertVolume(ctx, sqlc.UpsertVolumeParams{UserID: int64(userID), Volume: volume, TierLevel: int32(level)}))
}

func (r *FeeTierRepo) IncrementVolume(ctx context.Context, userID uint, delta float64) error {
	return wrap("increment volume", r.q.IncrementVolume(ctx, sqlc.IncrementVolumeParams{UserID: int64(userID), Delta: delta}))
}

func (r *FeeTierRepo) SeedDefaults(ctx context.Context) error {
	n, err := r.q.CountFeeTiers(ctx)
	if err != nil {
		return fmt.Errorf("postgres: count fee tiers: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, t := range domain.DefaultFeeTiers {
		if err := r.q.InsertFeeTier(ctx, sqlc.InsertFeeTierParams{
			Level: int32(t.Level), Name: t.Name, MinVolume: t.MinVolume, MakerFee: t.MakerFee, TakerFee: t.TakerFee, Description: t.Description,
		}); err != nil {
			return fmt.Errorf("postgres: seed fee tier: %w", err)
		}
	}
	return nil
}

func feeTierToDomain(m sqlc.FeeTier) domain.FeeTier {
	return domain.FeeTier{
		ID: uint(m.ID), Level: int(m.Level), Name: m.Name, MinVolume: m.MinVolume,
		MakerFee: m.MakerFee, TakerFee: m.TakerFee, Description: m.Description,
	}
}
