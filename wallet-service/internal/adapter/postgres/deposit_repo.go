package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/wallet-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/wallet-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DepositRepo is the pgx+sqlc adapter for domain.DepositRepository.
type DepositRepo struct{ pool *pgxpool.Pool }

func NewDepositRepo(pool *pgxpool.Pool) *DepositRepo { return &DepositRepo{pool: pool} }

var _ domain.DepositRepository = (*DepositRepo)(nil)

func (r *DepositRepo) Create(ctx context.Context, d *domain.Deposit) error {
	row, err := q(ctx, r.pool).CreateDeposit(ctx, sqlc.CreateDepositParams{
		UserID: int64(d.UserID), Amount: d.Amount, AmountUsdt: d.AmountUSDT, ExchangeRate: d.ExchangeRate,
		Currency: d.Currency, Method: d.Method, Status: string(d.Status), OrderCode: d.OrderCode,
		QrCodeUrl: d.QRCodeURL, SepayRef: d.SepayRef,
	})
	if err != nil {
		return fmt.Errorf("postgres: create deposit: %w", err)
	}
	d.ID = uint(row.ID)
	d.CreatedAt = row.CreatedAt
	d.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *DepositRepo) FindByID(ctx context.Context, id uint) (*domain.Deposit, error) {
	row, err := q(ctx, r.pool).GetDepositByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDepositNotFound
		}
		return nil, fmt.Errorf("postgres: get deposit by id: %w", err)
	}
	return depositToDomain(row), nil
}

func (r *DepositRepo) FindByOrderCode(ctx context.Context, code string) (*domain.Deposit, error) {
	row, err := q(ctx, r.pool).GetDepositByOrderCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDepositNotFound
		}
		return nil, fmt.Errorf("postgres: get deposit by order code: %w", err)
	}
	return depositToDomain(row), nil
}

func (r *DepositRepo) FindByUser(ctx context.Context, userID uint, page, size int) ([]domain.Deposit, int64, error) {
	qq := q(ctx, r.pool)
	rows, err := qq.ListDepositsByUser(ctx, sqlc.ListDepositsByUserParams{UserID: int64(userID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list deposits by user: %w", err)
	}
	total, err := qq.CountDepositsByUser(ctx, int64(userID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count deposits by user: %w", err)
	}
	return depositsToDomain(rows), total, nil
}

func (r *DepositRepo) Save(ctx context.Context, d *domain.Deposit) error {
	if err := q(ctx, r.pool).UpdateDeposit(ctx, sqlc.UpdateDepositParams{
		ID: int64(d.ID), Status: string(d.Status), AmountUsdt: d.AmountUSDT,
		ExchangeRate: d.ExchangeRate, QrCodeUrl: d.QRCodeURL, SepayRef: d.SepayRef,
	}); err != nil {
		return fmt.Errorf("postgres: update deposit: %w", err)
	}
	return nil
}

func (r *DepositRepo) ListAdmin(ctx context.Context, page, size int, search, status string) ([]domain.Deposit, int64, error) {
	qq := q(ctx, r.pool)
	rows, err := qq.ListDepositsAdmin(ctx, sqlc.ListDepositsAdminParams{
		Status: status, Search: search, Lim: int32(size), Off: int32((page - 1) * size),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list deposits admin: %w", err)
	}
	total, err := qq.CountDepositsAdmin(ctx, sqlc.CountDepositsAdminParams{Status: status, Search: search})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count deposits admin: %w", err)
	}
	return depositsToDomain(rows), total, nil
}

func depositToDomain(m sqlc.Deposit) *domain.Deposit {
	return &domain.Deposit{
		ID: uint(m.ID), UserID: uint(m.UserID), Amount: m.Amount, AmountUSDT: m.AmountUsdt,
		ExchangeRate: m.ExchangeRate, Currency: m.Currency, Method: m.Method,
		Status: domain.DepositStatus(m.Status), OrderCode: m.OrderCode, QRCodeURL: m.QrCodeUrl,
		SepayRef: m.SepayRef, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func depositsToDomain(ms []sqlc.Deposit) []domain.Deposit {
	out := make([]domain.Deposit, len(ms))
	for i, m := range ms {
		out[i] = *depositToDomain(m)
	}
	return out
}
