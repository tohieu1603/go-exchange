//go:build integration

// Integration tests for the pgx+sqlc trading adapters against a real Postgres
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

	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/trading-service/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("trading"), tcpostgres.WithUsername("trading"), tcpostgres.WithPassword("trading"),
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

func TestOrderRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewOrderRepo(pool)

	// A resting LIMIT buy.
	limit := &domain.Order{UserID: 1, Pair: "BTC_USDT", Side: domain.SideBuy, Type: domain.TypeLimit, Price: 50000, Amount: 2, Status: domain.StatusOpen}
	if err := repo.Create(ctx, limit); err != nil {
		t.Fatalf("Create limit: %v", err)
	}
	if limit.ID == 0 || limit.CreatedAt.IsZero() {
		t.Fatalf("Create must set ID + CreatedAt, got %+v", limit)
	}

	// A MARKET sell resting at price 0 (price seeded on fill).
	market := &domain.Order{UserID: 2, Pair: "BTC_USDT", Side: domain.SideSell, Type: domain.TypeMarket, Price: 0, Amount: 1, Status: domain.StatusOpen}
	if err := repo.Create(ctx, market); err != nil {
		t.Fatalf("Create market: %v", err)
	}

	// FindByID + owner-scoped lookup.
	got, err := repo.FindByID(ctx, limit.ID)
	if err != nil || got.Price != 50000 {
		t.Fatalf("FindByID = %+v, %v", got, err)
	}
	if _, err := repo.FindByUserAndID(ctx, 999, limit.ID); err != domain.ErrOrderNotFound {
		t.Fatalf("wrong-owner lookup must be ErrOrderNotFound, got %v", err)
	}
	if _, err := repo.FindByID(ctx, 123456); err != domain.ErrOrderNotFound {
		t.Fatalf("missing order must be ErrOrderNotFound, got %v", err)
	}

	// UpdateStatus seeds the MARKET price from the fill but never overwrites a
	// non-zero LIMIT price.
	if err := repo.UpdateStatus(ctx, market.ID, domain.StatusFilled, 1, 49000); err != nil {
		t.Fatalf("UpdateStatus market: %v", err)
	}
	if err := repo.UpdateStatus(ctx, limit.ID, domain.StatusPartial, 0.5, 51000); err != nil {
		t.Fatalf("UpdateStatus limit: %v", err)
	}
	if m, _ := repo.FindByID(ctx, market.ID); m.Price != 49000 || m.Status != domain.StatusFilled || m.FilledAmount != 1 {
		t.Fatalf("market fill not applied: %+v", m)
	}
	if l, _ := repo.FindByID(ctx, limit.ID); l.Price != 50000 || l.Status != domain.StatusPartial {
		t.Fatalf("limit price must be preserved (50000), got %+v", l)
	}

	// FindOpen returns OPEN+PARTIAL for the user; the limit is now PARTIAL.
	open, err := repo.FindOpen(ctx, 1)
	if err != nil || len(open) != 1 || open[0].ID != limit.ID {
		t.Fatalf("FindOpen(1) = %+v, %v", open, err)
	}
	// FindOpenLimitOrders returns only resting LIMIT orders (the PARTIAL limit).
	limits, err := repo.FindOpenLimitOrders(ctx)
	if err != nil || len(limits) != 1 || limits[0].Type != domain.TypeLimit {
		t.Fatalf("FindOpenLimitOrders = %+v, %v", limits, err)
	}

	// FindPaginated with + without status filter.
	all, total, err := repo.FindPaginated(ctx, 1, "", 1, 10)
	if err != nil || total != 1 || len(all) != 1 {
		t.Fatalf("FindPaginated(1, all) = len%d total%d %v", len(all), total, err)
	}
	none, total, err := repo.FindPaginated(ctx, 1, domain.StatusFilled, 1, 10)
	if err != nil || total != 0 || len(none) != 0 {
		t.Fatalf("FindPaginated(1, FILLED) should be empty, got len%d total%d %v", len(none), total, err)
	}

	// Save persists a full-row update.
	got.Status = domain.StatusCancelled
	got.FilledAmount = 0.5
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if reloaded, _ := repo.FindByID(ctx, got.ID); reloaded.Status != domain.StatusCancelled || reloaded.FilledAmount != 0.5 {
		t.Fatalf("Save did not persist: %+v", reloaded)
	}
}

func TestTradeRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewTradeRepo(pool)

	tr := &domain.Trade{
		Pair: "BTC_USDT", BuyOrderID: 10, SellOrderID: 11, BuyerID: 1, SellerID: 2,
		Price: 50000, Amount: 0.5, Total: 25000, BuyerFee: 25, SellerFee: 25,
	}
	if err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("Create trade: %v", err)
	}
	if tr.ID == 0 || tr.CreatedAt.IsZero() {
		t.Fatalf("Create must set ID + CreatedAt, got %+v", tr)
	}

	// Verify the row persisted with the expected numerics.
	var price, total, buyerFee float64
	var buyerID int64
	if err := pool.QueryRow(ctx, `SELECT price, total, buyer_fee, buyer_id FROM trades WHERE id=$1`, int64(tr.ID)).
		Scan(&price, &total, &buyerFee, &buyerID); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if price != 50000 || total != 25000 || buyerFee != 25 || buyerID != 1 {
		t.Fatalf("persisted trade mismatch: price=%v total=%v fee=%v buyer=%v", price, total, buyerFee, buyerID)
	}
}
