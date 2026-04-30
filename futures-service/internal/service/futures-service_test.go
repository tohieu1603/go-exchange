package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/cryptox/futures-service/internal/model"
	"github.com/cryptox/shared/ws"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ── fakes ──────────────────────────────────────────────────────────────────

// fakeWallet records every call so assertions can verify the Lock/Deduct
// ordering of OpenPosition + the Unlock/Credit/Deduct net of ClosePosition.
type fakeWallet struct {
	mu       sync.Mutex
	calls    []string
	failOn   string  // method name to return err on (eg "Lock", "Deduct")
	balance  float64 // simulated available
	locked   float64 // simulated locked
}

func (f *fakeWallet) record(name string, amount float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if f.failOn == name {
		return errors.New("forced " + name + " failure")
	}
	switch name {
	case "Lock":
		f.balance -= amount
		f.locked += amount
	case "Unlock":
		f.balance += amount
		f.locked -= amount
	case "Deduct":
		f.balance -= amount
	case "Credit":
		f.balance += amount
	}
	return nil
}

func (f *fakeWallet) CheckBalance(_ context.Context, _ uint, _ string, _ float64) error {
	return f.record("CheckBalance", 0)
}
func (f *fakeWallet) Deduct(_ context.Context, _ uint, _ string, a float64) error { return f.record("Deduct", a) }
func (f *fakeWallet) Credit(_ context.Context, _ uint, _ string, a float64) error { return f.record("Credit", a) }
func (f *fakeWallet) Lock(_ context.Context, _ uint, _ string, a float64) error   { return f.record("Lock", a) }
func (f *fakeWallet) Unlock(_ context.Context, _ uint, _ string, a float64) error { return f.record("Unlock", a) }
func (f *fakeWallet) GetBalance(_ context.Context, _ uint, _ string) (float64, float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "GetBalance")
	return f.balance, f.locked, nil
}

// fakePosRepo: in-memory position store. Only the methods OpenPosition +
// ClosePosition reach are implemented; others panic to surface drift.
type fakePosRepo struct {
	mu        sync.Mutex
	byID      map[uint]*model.FuturesPosition
	nextID    uint
	createErr error // injected to simulate DB failure on create
}

func newFakePosRepo() *fakePosRepo {
	return &fakePosRepo{byID: map[uint]*model.FuturesPosition{}, nextID: 1}
}
func (r *fakePosRepo) Create(_ *gorm.DB, p *model.FuturesPosition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	p.ID = r.nextID
	r.nextID++
	r.byID[p.ID] = p
	return nil
}
func (r *fakePosRepo) Save(_ *gorm.DB, p *model.FuturesPosition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[p.ID] = p
	return nil
}
func (r *fakePosRepo) FindByUserAndIDForUpdate(_ *gorm.DB, userID, id uint, status string) (*model.FuturesPosition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok || p.UserID != userID || (status != "" && p.Status != status) {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}
func (r *fakePosRepo) FindByIDForUpdate(_ *gorm.DB, _ uint, _ string) (*model.FuturesPosition, error) {
	panic("not used in these tests")
}
func (r *fakePosRepo) FindByUserAndID(_, _ uint, _ string) (*model.FuturesPosition, error) {
	panic("not used in these tests")
}
func (r *fakePosRepo) FindByUserAndStatus(_ uint, _ string) ([]model.FuturesPosition, error) {
	panic("not used in these tests")
}
func (r *fakePosRepo) FindOpenByUser(_ uint) ([]model.FuturesPosition, error) {
	panic("not used in these tests")
}
func (r *fakePosRepo) FindAllOpen() ([]model.FuturesPosition, error) {
	panic("not used in these tests")
}
func (r *fakePosRepo) UpdateTPSL(_, _ uint, _ map[string]interface{}) error {
	panic("not used in these tests")
}

// ── helpers ────────────────────────────────────────────────────────────────

// newSvc wires a FuturesService against fakes. Uses miniredis for the
// price-feed reads via go-redis. Returns the service plus the fakes for
// assertions, and a cleanup that closes miniredis.
func newSvc(t *testing.T, markPrice float64) (*FuturesService, *fakeWallet, *fakePosRepo, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	if markPrice > 0 {
		// FuturesService.getPrice reads "price:<pair>" as a float.
		mr.Set("price:BTC_USDT", "")
		_ = rdb.Set(context.Background(), "price:BTC_USDT", markPrice, 0).Err()
	}

	wallet := &fakeWallet{balance: 10000}
	repo := newFakePosRepo()
	hub := ws.NewHub(nil) // nil rdb → broadcasts buffer to localBcast (1024 slots)

	svc := NewFuturesService(repo, wallet, nil /*db unused on Open*/, rdb, hub, nil /*no bus*/)
	return svc, wallet, repo, func() { mr.Close() }
}

// ── OpenPosition ───────────────────────────────────────────────────────────

func TestOpenPosition_LocksMarginAndDeductsFee(t *testing.T) {
	svc, wallet, repo, cleanup := newSvc(t, 30000)
	defer cleanup()

	pos, err := svc.OpenPosition(1, OpenPositionReq{
		Pair: "BTC_USDT", Side: "LONG", Leverage: 10, Size: 0.1,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// notional = 0.1 * 30000 = 3000  → margin = 300, fee = 1.50
	if pos.Margin != 300 {
		t.Errorf("margin: want 300 got %f", pos.Margin)
	}
	// Lock(300) shifts margin from available → locked; Deduct(fee) removes
	// the trading fee from available. So:
	//   available = 10000 - 300 (locked) - 1.50 (fee) = 9698.50
	//   locked    = 300
	//   total equity = available + locked = 9998.50 (only fee actually lost)
	if wallet.balance != 10000-300-1.50 {
		t.Errorf("available after open: want 9698.50 got %f", wallet.balance)
	}
	if wallet.locked != 300 {
		t.Errorf("locked after open: want 300 got %f", wallet.locked)
	}
	if total := wallet.balance + wallet.locked; total != 10000-1.50 {
		t.Errorf("total equity should drop by fee only; want 9998.50 got %f", total)
	}
	// Position persisted.
	if _, ok := repo.byID[pos.ID]; !ok {
		t.Error("position not stored")
	}
	// Verify ordering: CheckBalance → Lock → Deduct.
	expectedPrefix := []string{"CheckBalance", "Lock", "Deduct"}
	for i, want := range expectedPrefix {
		if wallet.calls[i] != want {
			t.Errorf("call[%d]: want %s got %s (all=%v)", i, want, wallet.calls[i], wallet.calls)
		}
	}
}

func TestOpenPosition_PriceUnavailable_FailsBeforeWallet(t *testing.T) {
	svc, wallet, _, cleanup := newSvc(t, 0) // no price set
	defer cleanup()

	_, err := svc.OpenPosition(1, OpenPositionReq{
		Pair: "BTC_USDT", Side: "LONG", Leverage: 10, Size: 0.1,
	})
	if err == nil {
		t.Fatal("expected error when price unavailable")
	}
	if len(wallet.calls) != 0 {
		t.Errorf("wallet should not be touched; got calls=%v", wallet.calls)
	}
}

func TestOpenPosition_DeductFailureRollsBackLock(t *testing.T) {
	svc, wallet, repo, cleanup := newSvc(t, 30000)
	defer cleanup()
	wallet.failOn = "Deduct"

	_, err := svc.OpenPosition(1, OpenPositionReq{
		Pair: "BTC_USDT", Side: "LONG", Leverage: 10, Size: 0.1,
	})
	if err == nil {
		t.Fatal("expected error from forced Deduct failure")
	}
	// Lock fired then Deduct failed → must Unlock to restore state.
	hasUnlock := false
	for _, c := range wallet.calls {
		if c == "Unlock" {
			hasUnlock = true
		}
	}
	if !hasUnlock {
		t.Errorf("expected Unlock rollback after Deduct failure; calls=%v", wallet.calls)
	}
	// Margin must be released back to available.
	if wallet.locked != 0 {
		t.Errorf("locked should be 0 after rollback, got %f", wallet.locked)
	}
	// Position must NOT be persisted.
	if len(repo.byID) != 0 {
		t.Errorf("repo should be empty on failed open, got %d entries", len(repo.byID))
	}
}

func TestOpenPosition_RepoCreateFailureRefundsFeeAndUnlocks(t *testing.T) {
	svc, wallet, repo, cleanup := newSvc(t, 30000)
	defer cleanup()
	repo.createErr = errors.New("boom")

	_, err := svc.OpenPosition(1, OpenPositionReq{
		Pair: "BTC_USDT", Side: "LONG", Leverage: 10, Size: 0.1,
	})
	if err == nil {
		t.Fatal("expected error from forced repo failure")
	}
	// Wallet should be back to ~original after best-effort refund of fee + unlock.
	if wallet.locked != 0 {
		t.Errorf("locked should be 0 after rollback, got %f", wallet.locked)
	}
	if wallet.balance != 10000 {
		t.Errorf("balance should be restored to 10000, got %f", wallet.balance)
	}
}

// ── ClosePosition ──────────────────────────────────────────────────────────
//
// ClosePosition wraps the find+save in a gorm.Transaction, which calls fn(tx)
// against the embedded *gorm.DB. Passing a nil DB would panic — so we only
// test ClosePosition's settle math indirectly by asserting the wallet calls
// fired by an open-then-close sequence.
//
// To exercise the close path without a real DB, we'd need to mock
// *gorm.DB — out of scope. Service-level integration tests cover that.
//
// What we DO test here: the open path that backs up the close-side
// invariants — Lock + Deduct(fee) leaves margin in `locked`, ready for
// Unlock when close fires.
