// Package postgres implements domain.NotificationRepository on top of pgx + sqlc.
package postgres

import (
	"context"
	"fmt"

	"github.com/cryptox/notification-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/notification-service/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo adapts the sqlc-generated queries to the domain.NotificationRepository
// port, mapping rows to entities (int64 columns <-> uint domain ids).
type Repo struct {
	q *sqlc.Queries
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{q: sqlc.New(pool)} }

var _ domain.NotificationRepository = (*Repo)(nil)

func (r *Repo) Create(ctx context.Context, n *domain.Notification) error {
	row, err := r.q.CreateNotification(ctx, sqlc.CreateNotificationParams{
		UserID: int64(n.UserID), Type: n.Type, Title: n.Title, Message: n.Message, Pair: n.Pair,
	})
	if err != nil {
		return fmt.Errorf("postgres: create notification: %w", err)
	}
	n.ID = uint(row.ID)
	n.IsRead = row.IsRead
	n.CreatedAt = row.CreatedAt
	return nil
}

func (r *Repo) FindByUser(ctx context.Context, userID uint, unreadOnly bool, page, size int) ([]domain.Notification, int64, error) {
	limit := int32(size)
	offset := int32((page - 1) * size)
	var (
		rows  []sqlc.Notification
		total int64
		err   error
	)
	if unreadOnly {
		if rows, err = r.q.ListUnreadByUser(ctx, sqlc.ListUnreadByUserParams{UserID: int64(userID), Limit: limit, Offset: offset}); err == nil {
			total, err = r.q.CountUnreadByUser(ctx, int64(userID))
		}
	} else {
		if rows, err = r.q.ListByUser(ctx, sqlc.ListByUserParams{UserID: int64(userID), Limit: limit, Offset: offset}); err == nil {
			total, err = r.q.CountByUser(ctx, int64(userID))
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list notifications: %w", err)
	}
	return toDomains(rows), total, nil
}

func (r *Repo) UnreadCount(ctx context.Context, userID uint) (int64, error) {
	return r.q.CountUnreadByUser(ctx, int64(userID))
}

func (r *Repo) MarkAsRead(ctx context.Context, userID, notifID uint) error {
	return r.q.MarkAsRead(ctx, sqlc.MarkAsReadParams{ID: int64(notifID), UserID: int64(userID)})
}

func (r *Repo) MarkAllRead(ctx context.Context, userID uint) error {
	return r.q.MarkAllRead(ctx, int64(userID))
}

func toDomain(m sqlc.Notification) domain.Notification {
	return domain.Notification{
		ID: uint(m.ID), UserID: uint(m.UserID), Type: m.Type, Title: m.Title,
		Message: m.Message, Pair: m.Pair, IsRead: m.IsRead, CreatedAt: m.CreatedAt,
	}
}

func toDomains(ms []sqlc.Notification) []domain.Notification {
	out := make([]domain.Notification, len(ms))
	for i, m := range ms {
		out[i] = toDomain(m)
	}
	return out
}
