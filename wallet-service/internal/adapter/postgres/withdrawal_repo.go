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

// WithdrawalRepo is the pgx+sqlc adapter for domain.WithdrawalRepository.
type WithdrawalRepo struct{ pool *pgxpool.Pool }

func NewWithdrawalRepo(pool *pgxpool.Pool) *WithdrawalRepo { return &WithdrawalRepo{pool: pool} }

var _ domain.WithdrawalRepository = (*WithdrawalRepo)(nil)

func (r *WithdrawalRepo) Create(ctx context.Context, w *domain.Withdrawal) error {
	row, err := q(ctx, r.pool).CreateWithdrawal(ctx, sqlc.CreateWithdrawalParams{
		UserID: int64(w.UserID), Amount: w.Amount, Currency: w.Currency, BankCode: w.BankCode,
		BankAccount: w.BankAccount, AccountName: w.AccountName, Status: string(w.Status), AdminNote: w.AdminNote,
	})
	if err != nil {
		return fmt.Errorf("postgres: create withdrawal: %w", err)
	}
	w.ID = uint(row.ID)
	w.CreatedAt = row.CreatedAt
	w.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *WithdrawalRepo) FindByID(ctx context.Context, id uint) (*domain.Withdrawal, error) {
	row, err := q(ctx, r.pool).GetWithdrawalByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("postgres: get withdrawal by id: %w", err)
	}
	return withdrawalToDomain(row), nil
}

func (r *WithdrawalRepo) FindByUser(ctx context.Context, userID uint, page, size int) ([]domain.Withdrawal, int64, error) {
	qq := q(ctx, r.pool)
	rows, err := qq.ListWithdrawalsByUser(ctx, sqlc.ListWithdrawalsByUserParams{UserID: int64(userID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list withdrawals by user: %w", err)
	}
	total, err := qq.CountWithdrawalsByUser(ctx, int64(userID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count withdrawals by user: %w", err)
	}
	return withdrawalsToDomain(rows), total, nil
}

func (r *WithdrawalRepo) Save(ctx context.Context, w *domain.Withdrawal) error {
	if err := q(ctx, r.pool).UpdateWithdrawal(ctx, sqlc.UpdateWithdrawalParams{
		ID: int64(w.ID), Status: string(w.Status), AdminNote: w.AdminNote,
	}); err != nil {
		return fmt.Errorf("postgres: update withdrawal: %w", err)
	}
	return nil
}

func (r *WithdrawalRepo) FindLatestPendingByUser(ctx context.Context, userID uint) (*domain.Withdrawal, error) {
	row, err := q(ctx, r.pool).GetLatestPendingWithdrawal(ctx, int64(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("postgres: get latest pending withdrawal: %w", err)
	}
	return withdrawalToDomain(row), nil
}

func (r *WithdrawalRepo) ListAdmin(ctx context.Context, page, size int, search, status string) ([]domain.Withdrawal, int64, error) {
	qq := q(ctx, r.pool)
	rows, err := qq.ListWithdrawalsAdmin(ctx, sqlc.ListWithdrawalsAdminParams{
		Status: status, Search: search, Lim: int32(size), Off: int32((page - 1) * size),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list withdrawals admin: %w", err)
	}
	total, err := qq.CountWithdrawalsAdmin(ctx, sqlc.CountWithdrawalsAdminParams{Status: status, Search: search})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count withdrawals admin: %w", err)
	}
	return withdrawalsToDomain(rows), total, nil
}

func withdrawalToDomain(m sqlc.Withdrawal) *domain.Withdrawal {
	return &domain.Withdrawal{
		ID: uint(m.ID), UserID: uint(m.UserID), Amount: m.Amount, Currency: m.Currency,
		BankCode: m.BankCode, BankAccount: m.BankAccount, AccountName: m.AccountName,
		Status: domain.WithdrawalStatus(m.Status), AdminNote: m.AdminNote,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func withdrawalsToDomain(ms []sqlc.Withdrawal) []domain.Withdrawal {
	out := make([]domain.Withdrawal, len(ms))
	for i, m := range ms {
		out[i] = *withdrawalToDomain(m)
	}
	return out
}
