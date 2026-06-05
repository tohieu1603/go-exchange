package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/cryptox/trading-service/internal/domain"
)

type fakeOrderRepo struct {
	byID  map[uint]*domain.Order
	saved []*domain.Order
}

func newFakeOrderRepo() *fakeOrderRepo { return &fakeOrderRepo{byID: map[uint]*domain.Order{}} }

func (r *fakeOrderRepo) Create(_ context.Context, o *domain.Order) error { r.byID[o.ID] = o; return nil }
func (r *fakeOrderRepo) FindByID(_ context.Context, id uint) (*domain.Order, error) {
	if o, ok := r.byID[id]; ok {
		cp := *o
		return &cp, nil
	}
	return nil, domain.ErrOrderNotFound
}
func (r *fakeOrderRepo) FindByUserAndID(context.Context, uint, uint) (*domain.Order, error) {
	return nil, domain.ErrOrderNotFound
}
func (r *fakeOrderRepo) UpdateStatus(context.Context, uint, string, float64, float64) error {
	return nil
}
func (r *fakeOrderRepo) FindOpen(context.Context, uint) ([]domain.Order, error) { return nil, nil }
func (r *fakeOrderRepo) FindPaginated(context.Context, uint, string, int, int) ([]domain.Order, int64, error) {
	return nil, 0, nil
}
func (r *fakeOrderRepo) Save(_ context.Context, o *domain.Order) error {
	r.saved = append(r.saved, o)
	r.byID[o.ID] = o
	return nil
}
func (r *fakeOrderRepo) FindOpenLimitOrders(context.Context) ([]domain.Order, error) { return nil, nil }

type fakeBalanceCache struct{ unlocked map[string]float64 }

func newFakeBalanceCache() *fakeBalanceCache { return &fakeBalanceCache{unlocked: map[string]float64{}} }
func (c *fakeBalanceCache) Deduct(context.Context, uint, string, float64) (float64, error) {
	return 0, nil
}
func (c *fakeBalanceCache) Credit(context.Context, uint, string, float64) (float64, error) {
	return 0, nil
}
func (c *fakeBalanceCache) Lock(context.Context, uint, string, float64) error { return nil }
func (c *fakeBalanceCache) Unlock(_ context.Context, _ uint, currency string, amount float64) error {
	c.unlocked[currency] += amount
	return nil
}

func newOrderSvc() (*OrderService, *fakeOrderRepo, *fakeBalanceCache) {
	repo := newFakeOrderRepo()
	cache := newFakeBalanceCache()
	locker := NewBalanceLocker(cache, nil)
	return NewOrderService(repo, locker), repo, cache
}

func TestCancelOrder_Validations(t *testing.T) {
	svc, repo, _ := newOrderSvc()
	repo.byID[1] = &domain.Order{ID: 1, UserID: 2, Pair: "BTC_USDT", Type: domain.TypeLimit, Side: domain.SideBuy, Status: domain.StatusOpen}
	repo.byID[2] = &domain.Order{ID: 2, UserID: 1, Pair: "BTC_USDT", Type: domain.TypeLimit, Side: domain.SideBuy, Status: domain.StatusFilled}

	if _, err := svc.CancelOrder(context.Background(), 1, 99); !errors.Is(err, domain.ErrOrderNotFound) {
		t.Fatalf("missing order → ErrOrderNotFound, got %v", err)
	}
	if _, err := svc.CancelOrder(context.Background(), 1, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other user's order → ErrForbidden, got %v", err)
	}
	if _, err := svc.CancelOrder(context.Background(), 1, 2); !errors.Is(err, domain.ErrOrderNotCancelable) {
		t.Fatalf("filled order → ErrOrderNotCancelable, got %v", err)
	}
}

func TestCancelOrder_UnlocksLimitReservation(t *testing.T) {
	svc, repo, cache := newOrderSvc()
	// LIMIT BUY: price 100, amount 2 → reserved quote = 100*2*1.001 = 200.2
	repo.byID[1] = &domain.Order{ID: 1, UserID: 7, Pair: "BTC_USDT", Type: domain.TypeLimit, Side: domain.SideBuy, Price: 100, Amount: 2, Status: domain.StatusOpen}

	order, err := svc.CancelOrder(context.Background(), 7, 1)
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if order.Status != domain.StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", order.Status)
	}
	if got := cache.unlocked["USDT"]; got != 200.2 {
		t.Fatalf("unlocked USDT = %v, want 200.2", got)
	}
}
