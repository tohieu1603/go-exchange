// Package domain holds the notification bounded context: the Notification
// entity, its behaviour, and the repository port. Pure Go — no gorm/gin/redis.
package domain

import (
	"context"
	"time"
)

// Notification is a user-facing message produced by domain events elsewhere in
// the platform (order filled, deposit confirmed, position liquidated, …).
type Notification struct {
	ID        uint
	UserID    uint
	Type      string
	Title     string
	Message   string
	Pair      string
	IsRead    bool
	CreatedAt time.Time
}

// NewNotification creates an unread notification.
func NewNotification(userID uint, typ, title, message, pair string) *Notification {
	return &Notification{UserID: userID, Type: typ, Title: title, Message: message, Pair: pair, IsRead: false}
}

// MarkRead flips the notification to read.
func (n *Notification) MarkRead() { n.IsRead = true }

// NotificationRepository is the persistence port (owned by the domain;
// implemented by adapter/postgres over sqlc).
type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) error
	FindByUser(ctx context.Context, userID uint, unreadOnly bool, page, size int) ([]Notification, int64, error)
	UnreadCount(ctx context.Context, userID uint) (int64, error)
	MarkAsRead(ctx context.Context, userID, notifID uint) error
	MarkAllRead(ctx context.Context, userID uint) error
}

// Broadcaster pushes a real-time payload to a WebSocket channel (outbound port;
// shared/ws.Hub satisfies it directly).
type Broadcaster interface {
	Broadcast(channel string, data interface{})
}
