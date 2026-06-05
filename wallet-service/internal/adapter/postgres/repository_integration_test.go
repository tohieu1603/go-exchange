//go:build integration

// Integration tests for the pgx+sqlc wallet adapters and the pgx TxManager,
// against a real Postgres (Testcontainers). Run with:
//
//	go test -tags=integration ./...
//
// They apply the actual db/migrations schema and exercise the conditional
// balance UPDATEs, the deposit/withdrawal lifecycles, the admin read models
// (which JOIN a users table), and transaction commit/rollback semantics.
package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cryptox/wallet-service/internal/domain"
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
		tcpostgres.WithDatabase("wallet"), tcpostgres.WithUsername("wallet"), tcpostgres.WithPassword("wallet"),
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

	applyMigrations(t, ctx, pool)
	// The admin read models JOIN users (owned by auth-service in the shared DB);
	// create a minimal compatible table so the search path is testable here.
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS users (id bigint PRIMARY KEY, email text NOT NULL)`); err != nil {
		t.Fatalf("create users stub: %v", err)
	}
	return pool
}

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
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func TestWalletRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewWalletRepo(pool)

	const alice = uint(1)
	// CreateBatch provisions; a second insert of the same (user,currency) is a no-op.
	if err := repo.CreateBatch(ctx, []domain.Wallet{
		{UserID: alice, Currency: "USDT", Balance: 100},
		{UserID: alice, Currency: "VND", Balance: 0},
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if err := repo.CreateBatch(ctx, []domain.Wallet{{UserID: alice, Currency: "USDT", Balance: 999}}); err != nil {
		t.Fatalf("CreateBatch idempotent: %v", err)
	}

	if n, err := repo.CountByUser(ctx, alice); err != nil || n != 2 {
		t.Fatalf("CountByUser = %d, %v; want 2", n, err)
	}
	all, err := repo.FindAllByUser(ctx, alice)
	if err != nil || len(all) != 2 || all[0].Currency != "USDT" { // ordered by currency: USDT < VND
		t.Fatalf("FindAllByUser = %+v, %v", all, err)
	}
	w, err := repo.FindByUserAndCurrency(ctx, alice, "USDT")
	if err != nil || w.Balance != 100 {
		t.Fatalf("FindByUserAndCurrency USDT balance = %v, %v; want 100 (idempotent insert must NOT overwrite)", w, err)
	}
	if _, err := repo.FindByUserAndCurrency(ctx, alice, "BTC"); err != domain.ErrWalletNotFound {
		t.Fatalf("missing wallet must be ErrWalletNotFound, got %v", err)
	}

	// Credit + guarded debit.
	if err := repo.UpdateBalance(ctx, alice, "USDT", 50); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if err := repo.UpdateBalance(ctx, alice, "USDT", -30); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if w, _ := repo.FindByUserAndCurrency(ctx, alice, "USDT"); w.Balance != 120 {
		t.Fatalf("balance after +50 -30 = %v, want 120", w.Balance)
	}
	// Over-debit affects zero rows → silent no-op (Redis is the authoritative gate).
	if err := repo.UpdateBalance(ctx, alice, "USDT", -1000); err != nil {
		t.Fatalf("over-debit should not error: %v", err)
	}
	if w, _ := repo.FindByUserAndCurrency(ctx, alice, "USDT"); w.Balance != 120 {
		t.Fatalf("over-debit must not change balance, got %v", w.Balance)
	}

	// Lock: success then insufficient.
	if err := repo.Lock(ctx, alice, "USDT", 100); err != nil {
		t.Fatalf("Lock 100 of 120: %v", err)
	}
	if err := repo.Lock(ctx, alice, "USDT", 100); err != domain.ErrInsufficientBalance {
		t.Fatalf("Lock beyond available must be ErrInsufficientBalance, got %v", err)
	}
	if w, _ := repo.FindByUserAndCurrency(ctx, alice, "USDT"); w.Locked != 100 {
		t.Fatalf("locked = %v, want 100", w.Locked)
	}
	// Unlock floors at zero.
	if err := repo.Unlock(ctx, alice, "USDT", 250); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if w, _ := repo.FindByUserAndCurrency(ctx, alice, "USDT"); w.Locked != 0 {
		t.Fatalf("locked after over-unlock = %v, want 0", w.Locked)
	}

	// Upsert credits an existing row and creates a missing one.
	if err := repo.Upsert(ctx, alice, "USDT", 10); err != nil {
		t.Fatalf("Upsert existing: %v", err)
	}
	if err := repo.Upsert(ctx, alice, "ETH", 5); err != nil {
		t.Fatalf("Upsert new: %v", err)
	}
	if w, _ := repo.FindByUserAndCurrency(ctx, alice, "USDT"); w.Balance != 130 {
		t.Fatalf("USDT after upsert +10 = %v, want 130", w.Balance)
	}
	if w, err := repo.FindByUserAndCurrency(ctx, alice, "ETH"); err != nil || w.Balance != 5 {
		t.Fatalf("ETH created by upsert = %v, %v; want 5", w, err)
	}

	if rows, err := repo.ListAll(ctx); err != nil || len(rows) != 3 {
		t.Fatalf("ListAll = %d rows, %v; want 3", len(rows), err)
	}
}

func TestDepositRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewDepositRepo(pool)
	seedUser(t, ctx, pool, 7, "alice@example.com")
	seedUser(t, ctx, pool, 8, "bob@example.com")

	d, err := domain.NewDeposit(7, 250000, 10, 25000, "DEP-7-1", "qr://x")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == 0 || d.CreatedAt.IsZero() {
		t.Fatalf("Create must set ID + CreatedAt, got %+v", d)
	}

	got, err := repo.FindByOrderCode(ctx, "DEP-7-1")
	if err != nil || got.AmountUSDT != 10 || got.Status != domain.DepositPending {
		t.Fatalf("FindByOrderCode = %+v, %v", got, err)
	}
	if _, err := repo.FindByOrderCode(ctx, "nope"); err != domain.ErrDepositNotFound {
		t.Fatalf("missing order code must be ErrDepositNotFound, got %v", err)
	}

	// Confirm + Save persists the new status.
	if err := got.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	got.SepayRef = "BANK-REF-1"
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, _ := repo.FindByID(ctx, got.ID)
	if reloaded.Status != domain.DepositConfirmed || reloaded.SepayRef != "BANK-REF-1" {
		t.Fatalf("Save did not persist status/ref: %+v", reloaded)
	}

	// A bob deposit (PENDING) for the admin filters.
	d2, _ := domain.NewDeposit(8, 100000, 4, 25000, "DEP-8-1", "")
	_ = repo.Create(ctx, d2)

	list, total, err := repo.FindByUser(ctx, 7, 1, 10)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("FindByUser(7) = len%d total%d %v; want 1", len(list), total, err)
	}

	// Admin: status filter.
	pend, n, err := repo.ListAdmin(ctx, 1, 10, "", "PENDING")
	if err != nil || n != 1 || len(pend) != 1 || pend[0].UserID != 8 {
		t.Fatalf("ListAdmin PENDING = %+v n=%d %v; want bob's pending", pend, n, err)
	}
	// Admin: email search.
	byEmail, n, err := repo.ListAdmin(ctx, 1, 10, "alice", "")
	if err != nil || n != 1 || len(byEmail) != 1 || byEmail[0].UserID != 7 {
		t.Fatalf("ListAdmin search=alice = %+v n=%d %v; want alice's deposit", byEmail, n, err)
	}
}

func TestWithdrawalRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewWithdrawalRepo(pool)
	seedUser(t, ctx, pool, 7, "alice@example.com")

	w, err := domain.NewWithdrawal(7, 500000, "VCB", "999888", "ALICE")
	if err != nil {
		t.Fatalf("NewWithdrawal: %v", err)
	}
	if err := repo.Create(ctx, w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == 0 {
		t.Fatalf("Create must set ID")
	}

	latest, err := repo.FindLatestPendingByUser(ctx, 7)
	if err != nil || latest.ID != w.ID {
		t.Fatalf("FindLatestPendingByUser = %+v, %v", latest, err)
	}

	if err := w.Approve(); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	w.AdminNote = "paid"
	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.FindLatestPendingByUser(ctx, 7); err != domain.ErrWithdrawalNotFound {
		t.Fatalf("no pending after approve expected, got %v", err)
	}

	// Admin search by destination bank account.
	rows, n, err := repo.ListAdmin(ctx, 1, 10, "999888", "")
	if err != nil || n != 1 || len(rows) != 1 {
		t.Fatalf("ListAdmin search by bank account = %+v n=%d %v", rows, n, err)
	}
}

// TestTxManager_Integration proves commit persists and a returned error rolls
// back every write in the callback.
func TestTxManager_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	wallets := NewWalletRepo(pool)
	tm := NewTxManager(pool)

	if err := wallets.CreateBatch(ctx, []domain.Wallet{{UserID: 42, Currency: "USDT", Balance: 100}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Commit path.
	if err := tm.Do(ctx, func(c context.Context) error {
		return wallets.Upsert(c, 42, "USDT", 25)
	}); err != nil {
		t.Fatalf("commit Do: %v", err)
	}
	if w, _ := wallets.FindByUserAndCurrency(ctx, 42, "USDT"); w.Balance != 125 {
		t.Fatalf("after commit balance = %v, want 125", w.Balance)
	}

	// Rollback path: a callback error must discard the partial write.
	wantErr := domain.ErrInsufficientBalance
	if err := tm.Do(ctx, func(c context.Context) error {
		if err := wallets.Upsert(c, 42, "USDT", 1000); err != nil {
			return err
		}
		return wantErr // force rollback
	}); err != wantErr {
		t.Fatalf("Do should surface the callback error, got %v", err)
	}
	if w, _ := wallets.FindByUserAndCurrency(ctx, 42, "USDT"); w.Balance != 125 {
		t.Fatalf("rollback must discard write; balance = %v, want 125", w.Balance)
	}
}

func seedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, email string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1,$2) ON CONFLICT (id) DO NOTHING`, id, email); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}
