//go:build integration

// Integration tests for the pgx+sqlc candle adapter against a real Postgres
// (Testcontainers). Run with:  go test -tags=integration ./...
package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cryptox/market-service/internal/domain"
	"github.com/cryptox/shared/pgxdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("market"), tcpostgres.WithUsername("market"), tcpostgres.WithPassword("market"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	var pool *pgxpool.Pool
	for i := 0; i < 20; i++ {
		if pool, err = pgxdb.NewPool(ctx, dsn, 4); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	dir, _ := filepath.Abs(filepath.Join("..", "..", "..", "db", "migrations"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return pool
}

func TestCandleRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewCandleRepo(pool)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(min int, c float64) domain.Candle {
		return domain.Candle{Pair: "BTC_USDT", Interval: "1m", OpenTime: base.Add(time.Duration(min) * time.Minute),
			Open: c, High: c + 10, Low: c - 10, Close: c, Volume: 1}
	}

	// Single upsert, then overwrite the same bar (idempotent on pair+interval+open_time).
	if err := repo.Upsert(ctx, mk(0, 100)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	overwrite := mk(0, 999)
	if err := repo.Upsert(ctx, overwrite); err != nil {
		t.Fatalf("Upsert overwrite: %v", err)
	}

	// Batch upsert the rest (and re-send bar 0 to prove batch upsert path too).
	batch := []domain.Candle{mk(1, 101), mk(2, 102), mk(3, 103), mk(0, 1000)}
	if err := repo.UpsertBatch(ctx, batch); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	// Empty batch is a no-op.
	if err := repo.UpsertBatch(ctx, nil); err != nil {
		t.Fatalf("UpsertBatch(nil): %v", err)
	}

	// Query returns ascending by open_time, deduped to 4 distinct bars.
	got, err := repo.Query(ctx, "BTC_USDT", "1m", 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 distinct bars, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].OpenTime.After(got[i-1].OpenTime) {
			t.Fatalf("Query must be ascending by open_time: %v then %v", got[i-1].OpenTime, got[i].OpenTime)
		}
	}
	if got[0].Close != 1000 {
		t.Fatalf("bar 0 must reflect the last batch overwrite (1000), got %v", got[0].Close)
	}

	// Limit returns the most-recent N bars, still ascending.
	last2, err := repo.Query(ctx, "BTC_USDT", "1m", 2)
	if err != nil || len(last2) != 2 {
		t.Fatalf("Query limit 2 = len%d %v", len(last2), err)
	}
	// Compare instants with Equal (pgx returns the session-tz location, same instant).
	if !last2[0].OpenTime.Equal(base.Add(2*time.Minute)) || !last2[1].OpenTime.Equal(base.Add(3*time.Minute)) {
		t.Fatalf("limit 2 should be the two newest bars ascending, got %v / %v", last2[0].OpenTime, last2[1].OpenTime)
	}

	// A different (pair, interval) is isolated.
	if err := repo.Upsert(ctx, domain.Candle{Pair: "ETH_USDT", Interval: "1m", OpenTime: base, Close: 5}); err != nil {
		t.Fatalf("Upsert ETH: %v", err)
	}
	if eth, _ := repo.Query(ctx, "ETH_USDT", "1m", 100); len(eth) != 1 {
		t.Fatalf("ETH series should have 1 bar, got %d", len(eth))
	}
}
