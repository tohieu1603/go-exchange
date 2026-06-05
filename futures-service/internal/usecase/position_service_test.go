package usecase

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/cryptox/futures-service/internal/domain"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// ── fakes ───────────────────────────────────────────────────────────────────

type fakePositionRepo struct {
	byID map[uint]*domain.Position
	seq  uint
}

func newFakePositionRepo() *fakePositionRepo {
	return &fakePositionRepo{byID: map[uint]*domain.Position{}}
}
func (r *fakePositionRepo) Create(_ context.Context, p *domain.Position) error {
	r.seq++
	p.ID = r.seq
	cp := *p
	r.byID[p.ID] = &cp
	return nil
}
func (r *fakePositionRepo) FindOpenByUser(context.Context, uint) ([]domain.Position, error) {
	return nil, nil
}
func (r *fakePositionRepo) FindByUserAndStatus(context.Context, uint, string) ([]domain.Position, error) {
	return nil, nil
}
func (r *fakePositionRepo) FindAllOpen(context.Context) ([]domain.Position, error) { return nil, nil }
func (r *fakePositionRepo) FindByIDForUpdate(_ context.Context, id uint, status string) (*domain.Position, error) {
	if p, ok := r.byID[id]; ok && p.Status == status {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrPositionNotFound
}
func (r *fakePositionRepo) FindByUserAndIDForUpdate(_ context.Context, userID, id uint, status string) (*domain.Position, error) {
	if p, ok := r.byID[id]; ok && p.UserID == userID && p.Status == status {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrPositionNotFound
}
func (r *fakePositionRepo) Save(_ context.Context, p *domain.Position) error {
	cp := *p
	r.byID[p.ID] = &cp
	return nil
}
func (r *fakePositionRepo) UpdateTPSL(context.Context, uint, uint, *float64, *float64) error {
	return nil
}
func (r *fakePositionRepo) FindByUserAndID(_ context.Context, userID, id uint, status string) (*domain.Position, error) {
	return r.FindByUserAndIDForUpdate(context.Background(), userID, id, status)
}

type fakeWallet struct {
	balance                          float64
	locked, unlocked, credited, debited float64
	checkErr                         error
}

func (w *fakeWallet) CheckBalance(_ context.Context, _ uint, _ string, needed float64) error {
	if w.checkErr != nil {
		return w.checkErr
	}
	if needed > w.balance {
		return errors.New("insufficient")
	}
	return nil
}
func (w *fakeWallet) Deduct(_ context.Context, _ uint, _ string, a float64) error { w.debited += a; return nil }
func (w *fakeWallet) Credit(_ context.Context, _ uint, _ string, a float64) error { w.credited += a; return nil }
func (w *fakeWallet) Lock(_ context.Context, _ uint, _ string, a float64) error   { w.locked += a; return nil }
func (w *fakeWallet) Unlock(_ context.Context, _ uint, _ string, a float64) error { w.unlocked += a; return nil }
func (w *fakeWallet) GetBalance(context.Context, uint, string) (float64, float64, error) {
	return w.balance, w.locked, nil
}

type fakePrice struct{ mark float64 }

func (p fakePrice) MarkPrice(context.Context, string) (float64, bool)  { return p.mark, p.mark > 0 }
func (p fakePrice) IndexPrice(context.Context, string) (float64, bool) { return p.mark, p.mark > 0 }

type passthroughTx struct{}

func (passthroughTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type nopBroadcaster struct{}

func (nopBroadcaster) Broadcast(string, interface{}) {}

type nopBus struct{}

func (nopBus) Publish(context.Context, string, interface{}) error { return nil }

func newPositionSvc(wallet WalletGateway, price PriceSource) (*PositionService, *fakePositionRepo) {
	repo := newFakePositionRepo()
	return NewPositionService(repo, wallet, price, passthroughTx{}, nopBroadcaster{}, nopBus{}), repo
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestOpenPosition_LocksMarginAndChargesFee(t *testing.T) {
	wallet := &fakeWallet{balance: 1000}
	svc, repo := newPositionSvc(wallet, fakePrice{mark: 100})

	pos, err := svc.OpenPosition(context.Background(), 1, OpenCommand{Pair: "BTC_USDT", Side: domain.SideLong, Leverage: 10, Size: 1})
	if err != nil {
		t.Fatalf("OpenPosition: %v", err)
	}
	if !approx(wallet.locked, 10) { // notional 100 / 10
		t.Fatalf("locked = %v, want 10", wallet.locked)
	}
	if !approx(wallet.debited, 0.05) { // open fee 100*1*0.0005
		t.Fatalf("fee debited = %v, want 0.05", wallet.debited)
	}
	if repo.byID[pos.ID] == nil || repo.byID[pos.ID].Status != domain.StatusOpen {
		t.Fatalf("position not persisted OPEN")
	}
}

func TestOpenPosition_InsufficientBalanceDoesNotLock(t *testing.T) {
	wallet := &fakeWallet{balance: 1} // can't cover margin+fee
	svc, _ := newPositionSvc(wallet, fakePrice{mark: 100})

	if _, err := svc.OpenPosition(context.Background(), 1, OpenCommand{Pair: "BTC_USDT", Side: domain.SideLong, Leverage: 10, Size: 1}); err == nil {
		t.Fatal("expected insufficient-balance error")
	}
	if wallet.locked != 0 || wallet.debited != 0 {
		t.Fatalf("no funds should move on failed check: locked=%v debited=%v", wallet.locked, wallet.debited)
	}
}

func TestClosePosition_UnlocksAndSettles(t *testing.T) {
	wallet := &fakeWallet{balance: 1000}
	svc, repo := newPositionSvc(wallet, fakePrice{mark: 100})
	// Open at 100, then price rises to 110 before close.
	pos, err := svc.OpenPosition(context.Background(), 1, OpenCommand{Pair: "BTC_USDT", Side: domain.SideLong, Leverage: 10, Size: 1})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	svc.price = fakePrice{mark: 110}

	closed, err := svc.ClosePosition(context.Background(), 1, pos.ID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.Status != domain.StatusClosed {
		t.Fatalf("status = %s, want CLOSED", closed.Status)
	}
	if !approx(wallet.unlocked, 10) {
		t.Fatalf("unlocked = %v, want 10", wallet.unlocked)
	}
	if !approx(wallet.credited, 9.945) { // pnl 10 - closeFee 0.055
		t.Fatalf("credited = %v, want 9.945", wallet.credited)
	}
	if repo.byID[pos.ID].Status != domain.StatusClosed {
		t.Fatal("persisted position must be CLOSED")
	}
}

func TestClosePosition_NotFound(t *testing.T) {
	svc, _ := newPositionSvc(&fakeWallet{balance: 1000}, fakePrice{mark: 100})
	if _, err := svc.ClosePosition(context.Background(), 1, 999); !errors.Is(err, domain.ErrPositionNotFound) {
		t.Fatalf("want ErrPositionNotFound, got %v", err)
	}
}
