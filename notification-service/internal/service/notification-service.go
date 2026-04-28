package service

import (
	"fmt"
	"log"

	"github.com/cryptox/notification-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/notification-service/internal/model"
	"github.com/cryptox/shared/ws"
)

// NotificationService persists notification events and broadcasts via WebSocket hub.
// In the microservice architecture this service IS the Kafka consumer — it does not publish.
type NotificationService struct {
	repo repository.NotificationRepo
	hub  *ws.Hub
}

func NewNotificationService(repo repository.NotificationRepo, hub *ws.Hub) *NotificationService {
	return &NotificationService{repo: repo, hub: hub}
}

// PersistAndBroadcast is called by the Kafka consumer for each notification.created event.
// It persists the record to DB and fans out to the connected WebSocket client.
func (s *NotificationService) PersistAndBroadcast(event eventbus.NotificationEvent) {
	n := &model.Notification{
		UserID:  event.UserID,
		Type:    event.Type,
		Title:   event.Title,
		Message: event.Message,
		Pair:    event.Pair,
	}
	if err := s.repo.Create(nil, n); err != nil {
		log.Printf("[notification] persist error: %v", err)
		return
	}
	s.hub.Broadcast(fmt.Sprintf("notification@%d", n.UserID), map[string]interface{}{
		"id":        n.ID,
		"type":      n.Type,
		"title":     n.Title,
		"message":   n.Message,
		"pair":      n.Pair,
		"createdAt": n.CreatedAt,
	})
}

// GetUserNotifications returns paginated notifications for a user.
func (s *NotificationService) GetUserNotifications(userID uint, page, size int, unreadOnly bool) ([]model.Notification, int64, error) {
	return s.repo.FindByUser(userID, unreadOnly, page, size)
}

// UnreadCount returns the count of unread notifications for a user.
func (s *NotificationService) UnreadCount(userID uint) int64 {
	return s.repo.UnreadCount(userID)
}

// MarkAsRead marks a single notification as read.
func (s *NotificationService) MarkAsRead(userID, notifID uint) error {
	return s.repo.MarkAsRead(userID, notifID)
}

// MarkAllRead marks all notifications for a user as read.
func (s *NotificationService) MarkAllRead(userID uint) error {
	return s.repo.MarkAllRead(userID)
}
