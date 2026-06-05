// Package postgres implements the futures domain repositories on top of pgx +
// sqlc, plus a pgx-backed TxManager. The *ForUpdate reads issue SELECT ... FOR
// UPDATE and are only meaningful inside a transaction (carried on ctx).
package postgres

import (
	"context"

	"github.com/cryptox/futures-service/internal/adapter/postgres/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txKey is the private context key under which an active pgx.Tx is stored.
type txKey struct{}

// q returns a *sqlc.Queries bound to the transaction on ctx if one is active,
// otherwise to the base pool.
func q(ctx context.Context, pool *pgxpool.Pool) *sqlc.Queries {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok && tx != nil {
		return sqlc.New(tx)
	}
	return sqlc.New(pool)
}

// TxManager runs a callback inside a single pgx transaction. It structurally
// satisfies usecase.TxManager.
type TxManager struct{ pool *pgxpool.Pool }

func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// Do executes fn in a transaction (reusing an outer one if present). The FOR
// UPDATE row locks taken by repository reads inside fn serialise concurrent
// close/liquidate against the same position.
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
