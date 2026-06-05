// Package pgxdb provides a pgx connection pool and a golang-migrate runner —
// the appios-style replacement for gorm.Open. Repositories use sqlc-generated
// code over the *pgxpool.Pool returned here.
package pgxdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5 migrate driver
	_ "github.com/golang-migrate/migrate/v4/source/file"     // file:// migration source
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgx pool from dsn, applies pool limits + a statement
// timeout, and verifies connectivity with a bounded ping.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxdb: parse dsn: %w", err)
	}
	if maxConns > 0 {
		poolCfg.MaxConns = maxConns
	}
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	if _, ok := poolCfg.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000" // 30s
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pgxdb: new pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxdb: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies all up migrations under migrationsDir to dsn. "No change" is
// treated as success.
//
// migrationsTable names the bookkeeping table golang-migrate uses to track the
// applied version. Every go-exchange service shares ONE physical database, so
// each MUST pass a distinct table (e.g. "wallet_schema_migrations") — otherwise
// they would share the default "schema_migrations" and one service's version
// would make migrate skip another service's pending migrations. An empty value
// falls back to the library default.
func Migrate(dsn, migrationsDir, migrationsTable string) error {
	target := normalizeDSN(dsn)
	if migrationsTable != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "x-migrations-table=" + migrationsTable
	}
	m, err := migrate.New("file://"+migrationsDir, target)
	if err != nil {
		return fmt.Errorf("pgxdb: open migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("pgxdb: migrate up: %w", err)
	}
	return nil
}

// normalizeDSN rewrites postgres://|postgresql:// to the pgx5 migrate scheme.
func normalizeDSN(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgresql://"):
		return "pgx5://" + strings.TrimPrefix(dsn, "postgresql://")
	case strings.HasPrefix(dsn, "postgres://"):
		return "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	default:
		return dsn
	}
}
