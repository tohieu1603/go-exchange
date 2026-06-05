// Package postgres implements the wallet domain repositories on top of pgx +
// sqlc, plus a pgx-backed TxManager. It imports the domain (for ports/entities)
// and the generated sqlc package only — never gin or gRPC.
package postgres

import (
	"context"

	"github.com/cryptox/wallet-service/internal/adapter/postgres/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txKey is the private context key under which an active pgx.Tx is stored by
// TxManager and read back by the repository adapters.
type txKey struct{}

// q returns a *sqlc.Queries bound to the transaction on ctx if one is active,
// otherwise to the base pool. Every repository method routes through it so the
// same code path works inside and outside a transaction (mirrors the old
// gorm dbFrom helper).
func q(ctx context.Context, pool *pgxpool.Pool) *sqlc.Queries {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok && tx != nil {
		return sqlc.New(tx)
	}
	return sqlc.New(pool)
}

// TxManager runs a callback inside a single pgx transaction, threading the
// pgx.Tx through the context. It structurally satisfies usecase.TxManager.
type TxManager struct{ pool *pgxpool.Pool }

func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// Do executes fn in a transaction. A nested call (ctx already carries a tx)
// reuses the outer transaction rather than opening a new one, so use cases can
// compose without surprise savepoints.
func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok && tx != nil {
		return fn(ctx)
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
