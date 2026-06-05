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

// WalletRepo is the pgx+sqlc adapter for domain.WalletRepository.
//
// Balance mutations use conditional UPDATEs (generated as :execrows) so they are
// atomic under concurrency — never load-modify-write.
type WalletRepo struct{ pool *pgxpool.Pool }

func NewWalletRepo(pool *pgxpool.Pool) *WalletRepo { return &WalletRepo{pool: pool} }

var _ domain.WalletRepository = (*WalletRepo)(nil)

func (r *WalletRepo) FindByUserAndCurrency(ctx context.Context, userID uint, currency string) (*domain.Wallet, error) {
	row, err := q(ctx, r.pool).GetWallet(ctx, sqlc.GetWalletParams{UserID: int64(userID), Currency: currency})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, fmt.Errorf("postgres: get wallet: %w", err)
	}
	return &domain.Wallet{UserID: uint(row.UserID), Currency: row.Currency, Balance: row.Balance, Locked: row.LockedBalance}, nil
}

func (r *WalletRepo) FindAllByUser(ctx context.Context, userID uint) ([]domain.Wallet, error) {
	rows, err := q(ctx, r.pool).ListWalletsByUser(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: list wallets by user: %w", err)
	}
	out := make([]domain.Wallet, len(rows))
	for i, m := range rows {
		out[i] = domain.Wallet{UserID: uint(m.UserID), Currency: m.Currency, Balance: m.Balance, Locked: m.LockedBalance}
	}
	return out, nil
}

func (r *WalletRepo) ListAll(ctx context.Context) ([]domain.Wallet, error) {
	rows, err := q(ctx, r.pool).ListAllWallets(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list all wallets: %w", err)
	}
	out := make([]domain.Wallet, len(rows))
	for i, m := range rows {
		out[i] = domain.Wallet{UserID: uint(m.UserID), Currency: m.Currency, Balance: m.Balance, Locked: m.LockedBalance}
	}
	return out, nil
}

// UpdateBalance adds a signed delta. A debit that would overdraw matches zero
// rows and is a silent no-op (preserving the prior gorm behavior — the Redis hot
// path is the authoritative availability check).
func (r *WalletRepo) UpdateBalance(ctx context.Context, userID uint, currency string, delta float64) error {
	if _, err := q(ctx, r.pool).UpdateBalance(ctx, sqlc.UpdateBalanceParams{Delta: delta, UserID: int64(userID), Currency: currency}); err != nil {
		return fmt.Errorf("postgres: update balance: %w", err)
	}
	return nil
}

func (r *WalletRepo) Lock(ctx context.Context, userID uint, currency string, amount float64) error {
	n, err := q(ctx, r.pool).LockBalance(ctx, sqlc.LockBalanceParams{Amount: amount, UserID: int64(userID), Currency: currency})
	if err != nil {
		return fmt.Errorf("postgres: lock balance: %w", err)
	}
	if n == 0 {
		return domain.ErrInsufficientBalance
	}
	return nil
}

func (r *WalletRepo) Unlock(ctx context.Context, userID uint, currency string, amount float64) error {
	if err := q(ctx, r.pool).UnlockBalance(ctx, sqlc.UnlockBalanceParams{Amount: amount, UserID: int64(userID), Currency: currency}); err != nil {
		return fmt.Errorf("postgres: unlock balance: %w", err)
	}
	return nil
}

func (r *WalletRepo) IncreaseLocked(ctx context.Context, userID uint, currency string, amount float64) error {
	if err := q(ctx, r.pool).IncreaseLocked(ctx, sqlc.IncreaseLockedParams{Amount: amount, UserID: int64(userID), Currency: currency}); err != nil {
		return fmt.Errorf("postgres: increase locked: %w", err)
	}
	return nil
}

func (r *WalletRepo) CreateBatch(ctx context.Context, wallets []domain.Wallet) error {
	qq := q(ctx, r.pool)
	for _, w := range wallets {
		if err := qq.InsertWallet(ctx, sqlc.InsertWalletParams{UserID: int64(w.UserID), Currency: w.Currency, Balance: w.Balance}); err != nil {
			return fmt.Errorf("postgres: insert wallet: %w", err)
		}
	}
	return nil
}

func (r *WalletRepo) Upsert(ctx context.Context, userID uint, currency string, balance float64) error {
	if err := q(ctx, r.pool).UpsertWalletCredit(ctx, sqlc.UpsertWalletCreditParams{UserID: int64(userID), Currency: currency, Amount: balance}); err != nil {
		return fmt.Errorf("postgres: upsert wallet: %w", err)
	}
	return nil
}

func (r *WalletRepo) CountByUser(ctx context.Context, userID uint) (int64, error) {
	return q(ctx, r.pool).CountWalletsByUser(ctx, int64(userID))
}
