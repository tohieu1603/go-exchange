//go:build integration

// Integration tests for the pgx+sqlc futures adapters and the pgx TxManager
// (including SELECT ... FOR UPDATE locks) against a real Postgres
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

	"github.com/cryptox/futures-service/internal/domain"
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
		tcpostgres.WithDatabase("futures"), tcpostgres.WithUsername("futures"), tcpostgres.WithPassword("futures"),
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

func openPos(t *testing.T, ctx context.Context, repo *PositionRepo, userID uint) *domain.Position {
	t.Helper()
	p, err := domain.OpenPosition(userID, "BTC_USDT", domain.SideLong, 10, 1, 50000, 60000, 40000)
	if err != nil {
		t.Fatalf("OpenPosition: %v", err)
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create position: %v", err)
	}
	return p
}

func TestPositionRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewPositionRepo(pool)

	p := openPos(t, ctx, repo, 1)
	if p.ID == 0 || p.CreatedAt.IsZero() {
		t.Fatalf("Create must set ID + CreatedAt, got %+v", p)
	}

	// Reads.
	if open, err := repo.FindOpenByUser(ctx, 1); err != nil || len(open) != 1 {
		t.Fatalf("FindOpenByUser = %+v, %v", open, err)
	}
	if all, err := repo.FindByUserAndStatus(ctx, 1, ""); err != nil || len(all) != 1 {
		t.Fatalf("FindByUserAndStatus(all) = %+v, %v", all, err)
	}
	if none, err := repo.FindByUserAndStatus(ctx, 1, domain.StatusClosed); err != nil || len(none) != 0 {
		t.Fatalf("FindByUserAndStatus(CLOSED) should be empty, got %+v %v", none, err)
	}
	if allOpen, err := repo.FindAllOpen(ctx); err != nil || len(allOpen) != 1 {
		t.Fatalf("FindAllOpen = %+v, %v", allOpen, err)
	}
	if got, err := repo.FindByUserAndID(ctx, 1, p.ID, domain.StatusOpen); err != nil || got.Leverage != 10 {
		t.Fatalf("FindByUserAndID = %+v, %v", got, err)
	}
	if _, err := repo.FindByUserAndID(ctx, 999, p.ID, domain.StatusOpen); err != domain.ErrPositionNotFound {
		t.Fatalf("wrong owner must be ErrPositionNotFound, got %v", err)
	}

	// UpdateTPSL: update only TP (nil SL leaves it unchanged), then only SL.
	tp := 55000.0
	if err := repo.UpdateTPSL(ctx, p.ID, 1, &tp, nil); err != nil {
		t.Fatalf("UpdateTPSL tp-only: %v", err)
	}
	if g, _ := repo.FindByUserAndID(ctx, 1, p.ID, domain.StatusOpen); g.TakeProfit != 55000 || g.StopLoss != 40000 {
		t.Fatalf("after tp-only update: tp=%v sl=%v; want 55000/40000", g.TakeProfit, g.StopLoss)
	}
	sl := 45000.0
	if err := repo.UpdateTPSL(ctx, p.ID, 1, nil, &sl); err != nil {
		t.Fatalf("UpdateTPSL sl-only: %v", err)
	}
	if g, _ := repo.FindByUserAndID(ctx, 1, p.ID, domain.StatusOpen); g.TakeProfit != 55000 || g.StopLoss != 45000 {
		t.Fatalf("after sl-only update: tp=%v sl=%v; want 55000/45000", g.TakeProfit, g.StopLoss)
	}

	// Close + Save persists status, realized pnl and closed_at.
	got, _ := repo.FindByUserAndID(ctx, 1, p.ID, domain.StatusOpen)
	settle := got.Close(52000, time.Now())
	if settle.UnlockMargin <= 0 {
		t.Fatalf("Close should unlock margin, got %+v", settle)
	}
	if err := repo.Save(ctx, got); err != nil {
		t.Fatalf("Save: %v", err)
	}
	closed, err := repo.FindByUserAndID(ctx, 1, p.ID, domain.StatusClosed)
	if err != nil || closed.Status != domain.StatusClosed || closed.ClosedAt == nil {
		t.Fatalf("Save did not persist close: %+v, %v", closed, err)
	}
}

// TestPositionRepo_ForUpdate_InTx proves FindByIDForUpdate works inside a tx and
// that the surrounding tx commits/rolls back the subsequent Save.
func TestPositionRepo_ForUpdate_InTx(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewPositionRepo(pool)
	tm := NewTxManager(pool)
	p := openPos(t, ctx, repo, 2)

	// Rollback path: lock, mutate, then force an error → no change persists.
	boom := domain.ErrPriceUnavailable
	if err := tm.Do(ctx, func(c context.Context) error {
		locked, err := repo.FindByIDForUpdate(c, p.ID, domain.StatusOpen)
		if err != nil {
			return err
		}
		locked.Close(52000, time.Now())
		if err := repo.Save(c, locked); err != nil {
			return err
		}
		return boom
	}); err != boom {
		t.Fatalf("expected forced error, got %v", err)
	}
	if still, err := repo.FindByUserAndID(ctx, 2, p.ID, domain.StatusOpen); err != nil || still.Status != domain.StatusOpen {
		t.Fatalf("rollback must keep position OPEN, got %+v %v", still, err)
	}

	// Commit path.
	if err := tm.Do(ctx, func(c context.Context) error {
		locked, err := repo.FindByIDForUpdate(c, p.ID, domain.StatusOpen)
		if err != nil {
			return err
		}
		locked.Liquidate(48000, time.Now())
		return repo.Save(c, locked)
	}); err != nil {
		t.Fatalf("commit Do: %v", err)
	}
	if liq, err := repo.FindByUserAndID(ctx, 2, p.ID, domain.StatusLiquidated); err != nil || liq.Status != domain.StatusLiquidated {
		t.Fatalf("commit must persist LIQUIDATED, got %+v %v", liq, err)
	}
}

func TestFundingRepo_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewFundingRepo(pool)

	// No rate yet.
	if _, err := repo.LatestRate(ctx, "BTC_USDT"); err == nil {
		t.Fatalf("LatestRate on empty must error")
	}

	settled := time.Now().Truncate(time.Second)
	fr := &domain.FundingRate{Pair: "BTC_USDT", Rate: 0.0001, IndexPrice: 50000, MarkPrice: 50010, SettledAt: settled}
	if err := repo.CreateRate(ctx, fr); err != nil {
		t.Fatalf("CreateRate: %v", err)
	}
	firstID := fr.ID
	if firstID == 0 {
		t.Fatalf("CreateRate must set ID")
	}
	// Idempotent on (pair, settled_at): same id, not a duplicate.
	dup := &domain.FundingRate{Pair: "BTC_USDT", Rate: 0.0002, IndexPrice: 1, MarkPrice: 2, SettledAt: settled}
	if err := repo.CreateRate(ctx, dup); err != nil {
		t.Fatalf("CreateRate dup: %v", err)
	}
	if dup.ID != firstID {
		t.Fatalf("idempotent CreateRate should return same id %d, got %d", firstID, dup.ID)
	}

	latest, err := repo.LatestRate(ctx, "BTC_USDT")
	if err != nil || latest.Rate != 0.0001 { // original value preserved (no overwrite)
		t.Fatalf("LatestRate = %+v, %v; want rate 0.0001", latest, err)
	}
	if recent, err := repo.RecentRates(ctx, "BTC_USDT", 10); err != nil || len(recent) != 1 {
		t.Fatalf("RecentRates = %+v, %v", recent, err)
	}

	// Payments: idempotent on (position_id, funding_rate_id).
	pay := &domain.FundingPayment{PositionID: 1, UserID: 7, FundingRateID: firstID, Pair: "BTC_USDT", Side: domain.SideLong, Notional: 50000, Rate: 0.0001, Amount: -5}
	if err := repo.CreatePayment(ctx, pay); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if pay.ID == 0 || pay.CreatedAt.IsZero() {
		t.Fatalf("CreatePayment must set ID + CreatedAt, got %+v", pay)
	}
	dupPay := &domain.FundingPayment{PositionID: 1, UserID: 7, FundingRateID: firstID, Pair: "BTC_USDT", Side: domain.SideLong, Notional: 1, Rate: 1, Amount: 1}
	if err := repo.CreatePayment(ctx, dupPay); err != nil {
		t.Fatalf("CreatePayment dup: %v", err)
	}
	if dupPay.ID != pay.ID {
		t.Fatalf("idempotent CreatePayment should return same id %d, got %d", pay.ID, dupPay.ID)
	}

	hist, total, err := repo.HistoryByUser(ctx, 7, 1, 10)
	if err != nil || total != 1 || len(hist) != 1 || hist[0].Amount != -5 {
		t.Fatalf("HistoryByUser = %+v total=%d %v; want one payment of -5", hist, total, err)
	}
}
