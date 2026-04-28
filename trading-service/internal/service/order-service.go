package service

import (
	"context"
	"errors"
	"strings"

	"github.com/cryptox/trading-service/internal/model"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/trading-service/internal/repository"
)

// OrderService handles order persistence operations for the trading handler.
// The matching engine hot path is handled separately (in-memory + Redis).
type OrderService struct {
	orderRepo repository.OrderRepo
	balCache  *redisutil.BalanceCache
	locker    *BalanceLocker
}

func NewOrderService(orderRepo repository.OrderRepo, balCache *redisutil.BalanceCache, locker *BalanceLocker) *OrderService {
	return &OrderService{orderRepo: orderRepo, balCache: balCache, locker: locker}
}

func (s *OrderService) CreateOrder(order *model.Order) error {
	return s.orderRepo.Create(nil, order)
}

func (s *OrderService) UpdateOrderStatus(order *model.Order) {
	s.orderRepo.Save(nil, order)
}

// SyncOrderStatus updates status/filledAmount/price after engine processing
func (s *OrderService) SyncOrderStatus(order *model.Order) {
	s.orderRepo.UpdateStatus(nil, order.ID, order.Status, order.FilledAmount, order.Price)
}

// CancelOrder loads, validates, unlocks Redis balance, and marks order CANCELLED.
// Returns the cancelled order for the handler to remove from engine's book.
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uint) (*model.Order, error) {
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}
	if order.UserID != userID {
		return nil, errors.New("forbidden")
	}
	if order.Status != "OPEN" && order.Status != "PARTIAL" {
		return nil, errors.New("order cannot be cancelled")
	}

	parts := strings.Split(order.Pair, "_")
	base, quote := parts[0], parts[1]

	// Unlock Redis balance for LIMIT orders (hot path) and emit event so the
	// wallet projector decrements locked_balance in PostgreSQL.
	if order.Type == "LIMIT" {
		remaining := order.Remaining()
		if order.Side == "BUY" {
			unlockAmt := order.Price * remaining * 1.001
			s.locker.Unlock(ctx, userID, quote, unlockAmt)
		} else {
			s.locker.Unlock(ctx, userID, base, remaining)
		}
	}

	order.Status = "CANCELLED"
	if err := s.orderRepo.Save(nil, order); err != nil {
		return nil, err
	}

	return order, nil
}

func (s *OrderService) GetOrderHistory(userID uint, status string, page, size int) ([]model.Order, int64, error) {
	return s.orderRepo.FindPaginated(userID, status, page, size)
}

func (s *OrderService) GetOpenOrders(userID uint) ([]model.Order, error) {
	return s.orderRepo.FindOpen(userID)
}
