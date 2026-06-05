//go:build integration

// Integration test for the sqlc-backed NotificationRepository against a real
// Postgres (Testcontainers). Run with:  go test -tags=integration ./...
// It applies the actual db/migrations schema, then exercises every port method
// so the generated SQL, the int64<->uint mapping, and the owner-scoped updates
// are all validated end to end.
package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/cryptox/notification-service/internal/domain"
	"github.com/cryptox/shared/pgxdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestPool starts a throwaway Postgres, applies every up-migration, and
// returns a connected pool. The container is torn down via t.Cleanup.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("notif"),
		tcpostgres.WithUsername("notif"),
		tcpostgres.WithPassword("notif"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool := mustPool(t, ctx, dsn)
	applyMigrations(t, ctx, pool)
	return pool
}

func mustPool(t *testing.T, ctx context.Context, dsn string) *pgxpool.Pool {
	t.Helper()
	var pool *pgxpool.Pool
	var err error
	// Retry briefly: the container may report the port open a beat before the
	// server finishes its init/recovery and accepts connections.
	for i := 0; i < 20; i++ {
		pool, err = pgxdb.NewPool(ctx, dsn, 4)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// applyMigrations runs the real *.up.sql files in lexical order against the
// pool, exercising the production schema rather than a hand-rolled copy.
func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "db", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func TestRepo_Integration_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewRepo(newTestPool(t))

	const alice, bob = uint(1001), uint(2002)

	// Create populates the generated id/createdAt/isRead back onto the entity.
	n := domain.NewNotification(alice, "DEPOSIT_CONFIRMED", "Deposit", "1 BTC credited", "BTC_USDT")
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.ID == 0 {
		t.Fatalf("Create did not set ID")
	}
	if n.IsRead {
		t.Fatalf("new notification must be unread")
	}
	if n.CreatedAt.IsZero() {
		t.Fatalf("Create did not set CreatedAt")
	}

	// A second alice notification + one for bob (to prove user scoping).
	if err := repo.Create(ctx, domain.NewNotification(alice, "MARGIN_CALL", "Warning", "margin low", "ETH_USDT")); err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if err := repo.Create(ctx, domain.NewNotification(bob, "MARGIN_CALL", "Warning", "not yours", "ETH_USDT")); err != nil {
		t.Fatalf("Create bob: %v", err)
	}

	// FindByUser returns only alice's rows, newest first, with the true total.
	got, total, err := repo.FindByUser(ctx, alice, false, 1, 20)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if total != 2 || len(got) != 2 {
		t.Fatalf("expected 2 notifications for alice, got total=%d len=%d", total, len(got))
	}
	for _, g := range got {
		if g.UserID != alice {
			t.Fatalf("FindByUser leaked another user's row: %+v", g)
		}
	}

	// UnreadCount = 2 before any read.
	if c, err := repo.UnreadCount(ctx, alice); err != nil || c != 2 {
		t.Fatalf("UnreadCount before read = %d, %v; want 2, nil", c, err)
	}

	// MarkAsRead is owner-scoped: bob cannot flip alice's row.
	target := got[0].ID
	if err := repo.MarkAsRead(ctx, bob, target); err != nil {
		t.Fatalf("MarkAsRead (wrong owner) returned error: %v", err)
	}
	if c, _ := repo.UnreadCount(ctx, alice); c != 2 {
		t.Fatalf("bob's MarkAsRead must not affect alice; unread=%d want 2", c)
	}
	// Correct owner flips exactly one.
	if err := repo.MarkAsRead(ctx, alice, target); err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}
	if c, _ := repo.UnreadCount(ctx, alice); c != 1 {
		t.Fatalf("after one read, unread=%d want 1", c)
	}

	// unreadOnly filtering returns just the remaining unread row.
	unread, ut, err := repo.FindByUser(ctx, alice, true, 1, 20)
	if err != nil || ut != 1 || len(unread) != 1 || unread[0].IsRead {
		t.Fatalf("unread-only FindByUser = len%d total%d %v; want one unread row", len(unread), ut, err)
	}

	// MarkAllRead clears the rest for alice only.
	if err := repo.MarkAllRead(ctx, alice); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if c, _ := repo.UnreadCount(ctx, alice); c != 0 {
		t.Fatalf("after MarkAllRead, alice unread=%d want 0", c)
	}
	if c, _ := repo.UnreadCount(ctx, bob); c != 1 {
		t.Fatalf("bob's unread must be untouched=%d want 1", c)
	}
}
