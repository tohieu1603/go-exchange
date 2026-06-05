package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cryptox/shared/types"
	"github.com/cryptox/wallet-service/internal/domain"
)

// ── in-memory fakes for every port ───────────────────────────────────────────

func key(u uint, c string) string { return fmt.Sprintf("%d|%s", u, c) }

type fakeWalletRepo struct {
	bal    map[string]float64
	locked map[string]float64
	count  map[uint]int64
}

func newFakeWalletRepo() *fakeWalletRepo {
	return &fakeWalletRepo{bal: map[string]float64{}, locked: map[string]float64{}, count: map[uint]int64{}}
}
func (r *fakeWalletRepo) FindByUserAndCurrency(_ context.Context, u uint, c string) (*domain.Wallet, error) {
	b, ok := r.bal[key(u, c)]
	if !ok {
		return nil, domain.ErrWalletNotFound
	}
	return &domain.Wallet{UserID: u, Currency: c, Balance: b, Locked: r.locked[key(u, c)]}, nil
}
func (r *fakeWalletRepo) FindAllByUser(context.Context, uint) ([]domain.Wallet, error) { return nil, nil }
func (r *fakeWalletRepo) ListAll(context.Context) ([]domain.Wallet, error)             { return nil, nil }
func (r *fakeWalletRepo) UpdateBalance(_ context.Context, u uint, c string, d float64) error {
	if d < 0 && r.bal[key(u, c)] < -d {
		return nil // mirrors the conditional UPDATE affecting 0 rows
	}
	r.bal[key(u, c)] += d
	return nil
}
func (r *fakeWalletRepo) Lock(_ context.Context, u uint, c string, a float64) error {
	if r.bal[key(u, c)]-r.locked[key(u, c)] < a {
		return domain.ErrInsufficientBalance
	}
	r.locked[key(u, c)] += a
	return nil
}
func (r *fakeWalletRepo) Unlock(_ context.Context, u uint, c string, a float64) error {
	r.locked[key(u, c)] -= a
	if r.locked[key(u, c)] < 0 {
		r.locked[key(u, c)] = 0
	}
	return nil
}
func (r *fakeWalletRepo) IncreaseLocked(_ context.Context, u uint, c string, a float64) error {
	r.locked[key(u, c)] += a
	return nil
}
func (r *fakeWalletRepo) CreateBatch(_ context.Context, ws []domain.Wallet) error {
	for _, w := range ws {
		r.bal[key(w.UserID, w.Currency)] = w.Balance
		r.count[w.UserID]++
	}
	return nil
}
func (r *fakeWalletRepo) Upsert(_ context.Context, u uint, c string, b float64) error {
	r.bal[key(u, c)] += b
	return nil
}
func (r *fakeWalletRepo) CountByUser(_ context.Context, u uint) (int64, error) { return r.count[u], nil }

type fakeDepositRepo struct {
	byCode map[string]*domain.Deposit
	byID   map[uint]*domain.Deposit
	seq    uint
}

func newFakeDepositRepo() *fakeDepositRepo {
	return &fakeDepositRepo{byCode: map[string]*domain.Deposit{}, byID: map[uint]*domain.Deposit{}}
}
func (r *fakeDepositRepo) Create(_ context.Context, d *domain.Deposit) error {
	r.seq++
	d.ID = r.seq
	r.byCode[d.OrderCode] = d
	r.byID[d.ID] = d
	return nil
}
func (r *fakeDepositRepo) FindByID(_ context.Context, id uint) (*domain.Deposit, error) {
	if d, ok := r.byID[id]; ok {
		return d, nil
	}
	return nil, domain.ErrDepositNotFound
}
func (r *fakeDepositRepo) FindByOrderCode(_ context.Context, code string) (*domain.Deposit, error) {
	if d, ok := r.byCode[code]; ok {
		return d, nil
	}
	return nil, domain.ErrDepositNotFound
}
func (r *fakeDepositRepo) FindByUser(context.Context, uint, int, int) ([]domain.Deposit, int64, error) {
	return nil, 0, nil
}
func (r *fakeDepositRepo) Save(_ context.Context, d *domain.Deposit) error { r.byID[d.ID] = d; return nil }
func (r *fakeDepositRepo) ListAdmin(context.Context, int, int, string, string) ([]domain.Deposit, int64, error) {
	return nil, 0, nil
}

type fakeWithdrawalRepo struct {
	byID map[uint]*domain.Withdrawal
	seq  uint
}

func newFakeWithdrawalRepo() *fakeWithdrawalRepo {
	return &fakeWithdrawalRepo{byID: map[uint]*domain.Withdrawal{}}
}
func (r *fakeWithdrawalRepo) Create(_ context.Context, w *domain.Withdrawal) error {
	r.seq++
	w.ID = r.seq
	r.byID[w.ID] = w
	return nil
}
func (r *fakeWithdrawalRepo) FindByID(_ context.Context, id uint) (*domain.Withdrawal, error) {
	if w, ok := r.byID[id]; ok {
		return w, nil
	}
	return nil, domain.ErrWithdrawalNotFound
}
func (r *fakeWithdrawalRepo) FindByUser(context.Context, uint, int, int) ([]domain.Withdrawal, int64, error) {
	return nil, 0, nil
}
func (r *fakeWithdrawalRepo) Save(_ context.Context, w *domain.Withdrawal) error {
	r.byID[w.ID] = w
	return nil
}
func (r *fakeWithdrawalRepo) FindLatestPendingByUser(context.Context, uint) (*domain.Withdrawal, error) {
	return nil, domain.ErrWithdrawalNotFound
}
func (r *fakeWithdrawalRepo) ListAdmin(context.Context, int, int, string, string) ([]domain.Withdrawal, int64, error) {
	return nil, 0, nil
}

type passthroughTx struct{}

func (passthroughTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeCache struct {
	bal      map[string]float64
	locked   map[string]float64
	unlocked []string // records Unlock calls for assertions
}

func newFakeCache() *fakeCache {
	return &fakeCache{bal: map[string]float64{}, locked: map[string]float64{}}
}
func (c *fakeCache) GetBalance(_ context.Context, u uint, cur string) (float64, bool) {
	v, ok := c.bal[key(u, cur)]
	return v, ok
}
func (c *fakeCache) GetLocked(_ context.Context, u uint, cur string) float64 { return c.locked[key(u, cur)] }
func (c *fakeCache) LoadFromDB(_ context.Context, u uint, cur string, b, l float64) {
	c.bal[key(u, cur)] = b
	c.locked[key(u, cur)] = l
}
func (c *fakeCache) WarmIfMissing(_ context.Context, u uint, cur string, b, l float64) {
	if _, ok := c.bal[key(u, cur)]; !ok {
		c.bal[key(u, cur)] = b
		c.locked[key(u, cur)] = l
	}
}
func (c *fakeCache) Deduct(_ context.Context, u uint, cur string, a float64) (float64, error) {
	if c.bal[key(u, cur)] < a {
		return 0, domain.ErrInsufficientBalance
	}
	c.bal[key(u, cur)] -= a
	return c.bal[key(u, cur)], nil
}
func (c *fakeCache) Credit(_ context.Context, u uint, cur string, a float64) (float64, error) {
	c.bal[key(u, cur)] += a
	return c.bal[key(u, cur)], nil
}
func (c *fakeCache) Lock(_ context.Context, u uint, cur string, a float64) error {
	if c.bal[key(u, cur)]-c.locked[key(u, cur)] < a {
		return domain.ErrInsufficientBalance
	}
	c.locked[key(u, cur)] += a
	return nil
}
func (c *fakeCache) Unlock(_ context.Context, u uint, cur string, a float64) error {
	c.locked[key(u, cur)] -= a
	c.unlocked = append(c.unlocked, fmt.Sprintf("%d|%s|%.2f", u, cur, a))
	return nil
}

type fakeSettings struct {
	ps     types.PlatformSettings
	locked map[uint]bool
	bonus  map[uint]float64
}

func (s fakeSettings) PlatformSettings(context.Context) types.PlatformSettings { return s.ps }
func (s fakeSettings) AccountLocked(_ context.Context, u uint) bool             { return s.locked[u] }
func (s fakeSettings) BonusRemaining(_ context.Context, u uint) float64         { return s.bonus[u] }

type fixedRate struct{ r float64 }

func (f fixedRate) VNDRate() float64 { return f.r }

type recordPublisher struct{ events []string }

func (p *recordPublisher) Publish(_ context.Context, topic string, _ interface{}) error {
	p.events = append(p.events, topic)
	return nil
}

// harness wires the use case with fresh fakes.
type harness struct {
	uc       *WalletUseCase
	wallets  *fakeWalletRepo
	deposits *fakeDepositRepo
	withdraw *fakeWithdrawalRepo
	cache    *fakeCache
	settings *fakeSettings
	bus      *recordPublisher
}

func newHarness() *harness {
	wr, dr, wdr := newFakeWalletRepo(), newFakeDepositRepo(), newFakeWithdrawalRepo()
	cache := newFakeCache()
	set := &fakeSettings{locked: map[uint]bool{}, bonus: map[uint]float64{}}
	bus := &recordPublisher{}
	uc := NewWalletUseCase(wr, dr, wdr, passthroughTx{}, cache, set, fixedRate{r: 25000}, bus,
		SepayConfig{BankCode: "VCB", BankAccount: "123", AccountName: "EX"})
	return &harness{uc: uc, wallets: wr, deposits: dr, withdraw: wdr, cache: cache, settings: set, bus: bus}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestCreateDeposit_EnforcesMinAndLock(t *testing.T) {
	h := newHarness()
	h.settings.ps = types.PlatformSettings{MinDeposit: 100000}

	if _, err := h.uc.CreateDeposit(context.Background(), 1, 50000); !errors.Is(err, domain.ErrAmountOutOfRange) {
		t.Fatalf("below-minimum deposit must be rejected, got %v", err)
	}
	h.settings.locked[1] = true
	if _, err := h.uc.CreateDeposit(context.Background(), 1, 200000); !errors.Is(err, domain.ErrAccountLocked) {
		t.Fatalf("locked account must be rejected, got %v", err)
	}
}

func TestConfirmDeposit_CreditsOnceThenIdempotent(t *testing.T) {
	h := newHarness()
	d, err := h.uc.CreateDeposit(context.Background(), 7, 250000) // 250000/25000 = 10 USDT
	if err != nil {
		t.Fatalf("CreateDeposit: %v", err)
	}
	if err := h.uc.ConfirmDeposit(context.Background(), d.OrderCode); err != nil {
		t.Fatalf("ConfirmDeposit: %v", err)
	}
	if got := h.wallets.bal[key(7, "USDT")]; got != 10 {
		t.Fatalf("USDT balance after confirm = %v, want 10", got)
	}
	if got := h.cache.bal[key(7, "USDT")]; got != 10 {
		t.Fatalf("cache USDT after confirm = %v, want 10", got)
	}
	// Re-confirming the same order must fail (idempotency via state machine).
	if err := h.uc.ConfirmDeposit(context.Background(), d.OrderCode); !errors.Is(err, domain.ErrDepositNotPending) {
		t.Fatalf("re-confirm must error, got %v", err)
	}
}

func TestCreateWithdrawal_LocksFundsAndRespectsBonusCap(t *testing.T) {
	h := newHarness()
	// Seed 100 USDT in both DB and cache.
	h.wallets.bal[key(5, "USDT")] = 100
	h.cache.bal[key(5, "USDT")] = 100

	w, err := h.uc.CreateWithdrawal(context.Background(), 5, 40, "VCB", "123", "ALICE")
	if err != nil {
		t.Fatalf("CreateWithdrawal: %v", err)
	}
	if w.Status != domain.WithdrawalPending {
		t.Fatalf("new withdrawal must be PENDING")
	}
	if h.cache.locked[key(5, "USDT")] != 40 || h.wallets.locked[key(5, "USDT")] != 40 {
		t.Fatalf("40 USDT should be locked in cache+db, got cache=%v db=%v",
			h.cache.locked[key(5, "USDT")], h.wallets.locked[key(5, "USDT")])
	}

	// 50 USDT is bonus-locked: only 100-40-50 = 10 withdrawable now.
	h.settings.bonus[5] = 50
	if _, err := h.uc.CreateWithdrawal(context.Background(), 5, 20, "VCB", "123", "ALICE"); !errors.Is(err, domain.ErrAmountOutOfRange) {
		t.Fatalf("withdrawal beyond bonus cap must be rejected, got %v", err)
	}
}

func TestCreateWithdrawal_InsufficientBalance(t *testing.T) {
	h := newHarness()
	h.cache.bal[key(9, "USDT")] = 5
	if _, err := h.uc.CreateWithdrawal(context.Background(), 9, 10, "VCB", "1", "A"); !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", err)
	}
}

// AdminRejectWithdrawal must release the reservation in BOTH the DB and the
// Redis cache — the regression this asserts against.
func TestAdminRejectWithdrawal_UnlocksCacheAndDB(t *testing.T) {
	h := newHarness()
	h.wallets.bal[key(3, "USDT")] = 100
	h.cache.bal[key(3, "USDT")] = 100
	w, err := h.uc.CreateWithdrawal(context.Background(), 3, 30, "VCB", "123", "A")
	if err != nil {
		t.Fatalf("CreateWithdrawal: %v", err)
	}

	if err := h.uc.AdminRejectWithdrawal(context.Background(), w.ID); err != nil {
		t.Fatalf("AdminRejectWithdrawal: %v", err)
	}
	if h.wallets.locked[key(3, "USDT")] != 0 {
		t.Fatalf("DB lock not released: %v", h.wallets.locked[key(3, "USDT")])
	}
	if h.cache.locked[key(3, "USDT")] != 0 {
		t.Fatalf("cache lock not released (the bug): %v", h.cache.locked[key(3, "USDT")])
	}
	if err := h.uc.AdminApproveWithdrawal(context.Background(), w.ID); err == nil {
		t.Fatal("approving an already-rejected withdrawal must error")
	}
}

func TestProvisionWallets_Idempotent(t *testing.T) {
	h := newHarness()
	if err := h.uc.ProvisionWallets(context.Background(), 11, "default"); err != nil {
		t.Fatalf("ProvisionWallets: %v", err)
	}
	first := h.wallets.count[11]
	if first == 0 {
		t.Fatal("expected wallets to be provisioned")
	}
	// Second call is a no-op (user already has wallets).
	if err := h.uc.ProvisionWallets(context.Background(), 11, "default"); err != nil {
		t.Fatalf("second ProvisionWallets: %v", err)
	}
	if h.wallets.count[11] != first {
		t.Fatalf("provisioning must be idempotent, count grew %d -> %d", first, h.wallets.count[11])
	}
}
