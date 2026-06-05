// Package postgres implements the trading domain repositories on top of pgx +
// sqlc. It imports the domain (ports/entities) and the generated sqlc package
// only — never gin or gRPC.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/trading-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/trading-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderRepo is the pgx+sqlc adapter for domain.OrderRepository.
type OrderRepo struct{ q *sqlc.Queries }

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo { return &OrderRepo{q: sqlc.New(pool)} }

var _ domain.OrderRepository = (*OrderRepo)(nil)

func (r *OrderRepo) Create(ctx context.Context, order *domain.Order) error {
	row, err := r.q.CreateOrder(ctx, sqlc.CreateOrderParams{
		UserID: int64(order.UserID), Pair: order.Pair, Side: order.Side, Type: order.Type,
		Price: order.Price, StopPrice: order.StopPrice, Amount: order.Amount,
		FilledAmount: order.FilledAmount, Status: order.Status,
	})
	if err != nil {
		return fmt.Errorf("postgres: create order: %w", err)
	}
	order.ID = uint(row.ID)
	order.CreatedAt = row.CreatedAt
	order.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *OrderRepo) FindByID(ctx context.Context, id uint) (*domain.Order, error) {
	row, err := r.q.GetOrderByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, fmt.Errorf("postgres: get order by id: %w", err)
	}
	o := orderToDomain(row)
	return &o, nil
}

func (r *OrderRepo) FindByUserAndID(ctx context.Context, userID, orderID uint) (*domain.Order, error) {
	row, err := r.q.GetOrderByUserAndID(ctx, sqlc.GetOrderByUserAndIDParams{ID: int64(orderID), UserID: int64(userID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, fmt.Errorf("postgres: get order by user+id: %w", err)
	}
	o := orderToDomain(row)
	return &o, nil
}

func (r *OrderRepo) UpdateStatus(ctx context.Context, id uint, status string, filledAmount, price float64) error {
	if err := r.q.UpdateOrderStatus(ctx, sqlc.UpdateOrderStatusParams{
		ID: int64(id), Status: status, FilledAmount: filledAmount, Price: price,
	}); err != nil {
		return fmt.Errorf("postgres: update order status: %w", err)
	}
	return nil
}

func (r *OrderRepo) FindOpen(ctx context.Context, userID uint) ([]domain.Order, error) {
	rows, err := r.q.FindOpenOrders(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: find open orders: %w", err)
	}
	return ordersToDomain(rows), nil
}

func (r *OrderRepo) FindPaginated(ctx context.Context, userID uint, status string, page, size int) ([]domain.Order, int64, error) {
	rows, err := r.q.ListOrdersByUser(ctx, sqlc.ListOrdersByUserParams{
		UserID: int64(userID), Status: status, Lim: int32(size), Off: int32((page - 1) * size),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list orders by user: %w", err)
	}
	total, err := r.q.CountOrdersByUser(ctx, sqlc.CountOrdersByUserParams{UserID: int64(userID), Status: status})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count orders by user: %w", err)
	}
	return ordersToDomain(rows), total, nil
}

func (r *OrderRepo) Save(ctx context.Context, order *domain.Order) error {
	if err := r.q.SaveOrder(ctx, sqlc.SaveOrderParams{
		ID: int64(order.ID), Pair: order.Pair, Side: order.Side, Type: order.Type,
		Price: order.Price, StopPrice: order.StopPrice, Amount: order.Amount,
		FilledAmount: order.FilledAmount, Status: order.Status,
	}); err != nil {
		return fmt.Errorf("postgres: save order: %w", err)
	}
	return nil
}

func (r *OrderRepo) FindOpenLimitOrders(ctx context.Context) ([]domain.Order, error) {
	rows, err := r.q.FindOpenLimitOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: find open limit orders: %w", err)
	}
	return ordersToDomain(rows), nil
}

func orderToDomain(m sqlc.Order) domain.Order {
	return domain.Order{
		ID: uint(m.ID), UserID: uint(m.UserID), Pair: m.Pair, Side: m.Side, Type: m.Type,
		Price: m.Price, StopPrice: m.StopPrice, Amount: m.Amount, FilledAmount: m.FilledAmount,
		Status: m.Status, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func ordersToDomain(ms []sqlc.Order) []domain.Order {
	out := make([]domain.Order, len(ms))
	for i, m := range ms {
		out[i] = orderToDomain(m)
	}
	return out
}
