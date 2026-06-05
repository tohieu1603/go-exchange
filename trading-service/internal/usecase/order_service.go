package usecase

import (
	"context"
	"strings"

	"github.com/cryptox/trading-service/internal/domain"
)

// OrderService handles order persistence + cancellation (the matching hot path
// lives in MatchingEngine).
type OrderService struct {
	orderRepo domain.OrderRepository
	locker    *BalanceLocker
}

func NewOrderService(orderRepo domain.OrderRepository, locker *BalanceLocker) *OrderService {
	return &OrderService{orderRepo: orderRepo, locker: locker}
}

func (s *OrderService) CreateOrder(ctx context.Context, order *domain.Order) error {
	return s.orderRepo.Create(ctx, order)
}

func (s *OrderService) UpdateOrderStatus(ctx context.Context, order *domain.Order) {
	_ = s.orderRepo.Save(ctx, order)
}

// SyncOrderStatus persists status/filled/price after engine processing.
func (s *OrderService) SyncOrderStatus(ctx context.Context, order *domain.Order) {
	_ = s.orderRepo.UpdateStatus(ctx, order.ID, order.Status, order.FilledAmount, order.Price)
}

// CancelOrder validates ownership/state, releases any LIMIT reservation, and
// marks the order CANCELLED. Returns the order for the caller to remove from the
// in-memory book.
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uint) (*domain.Order, error) {
	order, err := s.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, domain.ErrOrderNotFound
	}
	if order.UserID != userID {
		return nil, domain.ErrForbidden
	}
	if !order.IsCancellable() {
		return nil, domain.ErrOrderNotCancelable
	}

	if order.Type == domain.TypeLimit {
		parts := strings.Split(order.Pair, "_")
		if len(parts) == 2 {
			base, quote := parts[0], parts[1]
			remaining := order.Remaining()
			if order.Side == domain.SideBuy {
				s.locker.Unlock(ctx, userID, quote, order.Price*remaining*1.001)
			} else {
				s.locker.Unlock(ctx, userID, base, remaining)
			}
		}
	}

	order.Status = domain.StatusCancelled
	if err := s.orderRepo.Save(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderService) GetOrderHistory(ctx context.Context, userID uint, status string, page, size int) ([]domain.Order, int64, error) {
	return s.orderRepo.FindPaginated(ctx, userID, status, page, size)
}

func (s *OrderService) GetOpenOrders(ctx context.Context, userID uint) ([]domain.Order, error) {
	return s.orderRepo.FindOpen(ctx, userID)
}
