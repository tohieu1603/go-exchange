package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/cryptox/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsRepo is the pgx+sqlc adapter for usecase.SettingsRepo (the single-row
// platform_settings table). It speaks types.PlatformSettings, the shared shape
// the rest of the platform consumes from Redis.
type SettingsRepo struct{ q *sqlc.Queries }

func NewSettingsRepo(pool *pgxpool.Pool) *SettingsRepo { return &SettingsRepo{q: sqlc.New(pool)} }

var _ usecase.SettingsRepo = (*SettingsRepo)(nil)

func (r *SettingsRepo) Get(ctx context.Context) (*types.PlatformSettings, error) {
	row, err := r.q.GetPlatformSettings(ctx, 1)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get platform settings: %w", err)
	}
	return &types.PlatformSettings{
		ID: uint(row.ID), DepositFeePercent: row.DepositFeePercent, WithdrawFeePercent: row.WithdrawFeePercent,
		MinDeposit: row.MinDeposit, MaxDeposit: row.MaxDeposit, MinWithdraw: row.MinWithdraw,
		MaxWithdraw: row.MaxWithdraw, TradingFeePercent: row.TradingFeePercent, KYCRequired: row.KycRequired,
	}, nil
}

func (r *SettingsRepo) Upsert(ctx context.Context, s *types.PlatformSettings) error {
	return wrap("upsert platform settings", r.q.UpsertPlatformSettings(ctx, sqlc.UpsertPlatformSettingsParams{
		ID: 1, DepositFeePercent: s.DepositFeePercent, WithdrawFeePercent: s.WithdrawFeePercent,
		MinDeposit: s.MinDeposit, MaxDeposit: s.MaxDeposit, MinWithdraw: s.MinWithdraw,
		MaxWithdraw: s.MaxWithdraw, TradingFeePercent: s.TradingFeePercent, KycRequired: s.KYCRequired,
	}))
}
