// Package usecase holds the notification application business rules. It depends
// only on the domain layer (repository + Broadcaster ports).
package usecase

import (
	"context"
	"fmt"

	"github.com/cryptox/notification-service/internal/domain"
)

// NotificationUseCase persists notifications and fans them out over WebSocket.
type NotificationUseCase struct {
	repo        domain.NotificationRepository
	broadcaster domain.Broadcaster
}

func NewNotificationUseCase(repo domain.NotificationRepository, b domain.Broadcaster) *NotificationUseCase {
	return &NotificationUseCase{repo: repo, broadcaster: b}
}

// Notify persists a notification then pushes it to the owner's private channel.
// Called by the notification.created consumer for each inbound event.
func (uc *NotificationUseCase) Notify(ctx context.Context, userID uint, typ, title, message, pair string) error {
	n := domain.NewNotification(userID, typ, title, message, pair)
	if err := uc.repo.Create(ctx, n); err != nil {
		return err
	}
	if uc.broadcaster != nil {
		uc.broadcaster.Broadcast(fmt.Sprintf("notification@%d", n.UserID), map[string]interface{}{
			"id": n.ID, "type": n.Type, "title": n.Title, "message": n.Message,
			"pair": n.Pair, "createdAt": n.CreatedAt,
		})
	}
	return nil
}

func (uc *NotificationUseCase) List(ctx context.Context, userID uint, page, size int, unreadOnly bool) ([]domain.Notification, int64, error) {
	return uc.repo.FindByUser(ctx, userID, unreadOnly, page, size)
}

func (uc *NotificationUseCase) UnreadCount(ctx context.Context, userID uint) (int64, error) {
	return uc.repo.UnreadCount(ctx, userID)
}

func (uc *NotificationUseCase) MarkRead(ctx context.Context, userID, notifID uint) error {
	return uc.repo.MarkAsRead(ctx, userID, notifID)
}

func (uc *NotificationUseCase) MarkAllRead(ctx context.Context, userID uint) error {
	return uc.repo.MarkAllRead(ctx, userID)
}
