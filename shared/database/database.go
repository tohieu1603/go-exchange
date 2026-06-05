// Package database centralizes GORM/PostgreSQL connection setup so every
// service opens its database the same way: with a configured connection pool,
// a startup connectivity check, and consistent logging.
//
// Before this package existed each service hand-rolled
//
//	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
//
// which silently skipped the shared connection-pool configuration
// (config.ConfigureDBPool was defined but never called), leaving GORM's
// default of MaxIdleConns=2 / MaxOpenConns=unlimited in place on all six
// PostgreSQL-backed services.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cryptox/shared/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// pingTimeout bounds the startup connectivity check so a misconfigured or
// unreachable database fails fast instead of blocking the boot indefinitely.
const pingTimeout = 5 * time.Second

// Open opens a GORM PostgreSQL connection using cfg.DSN, applies the shared
// connection-pool limits (config.ConfigureDBPool), and verifies connectivity
// with a bounded ping. The returned *gorm.DB is ready for AutoMigrate and use.
func Open(cfg config.BaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		// Warn keeps slow-query and error logs while dropping the noisy
		// per-statement Info logging that GORM emits by default.
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("retrieve sql.DB handle: %w", err)
	}
	config.ConfigureDBPool(sqlDB)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// MustOpen is Open but terminates the process on failure. A service that cannot
// reach its own database cannot serve traffic, so failing fast at boot is the
// correct behaviour for every cmd/main.go.
func MustOpen(cfg config.BaseConfig) *gorm.DB {
	db, err := Open(cfg)
	if err != nil {
		panic(fmt.Sprintf("database.MustOpen: %v", err))
	}
	return db
}
