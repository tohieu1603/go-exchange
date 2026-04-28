package repository

import (
	"github.com/cryptox/notification-service/internal/model"
	"gorm.io/gorm"
)

// NotificationRepo defines persistence operations for notifications.
type NotificationRepo interface {
	Create(tx *gorm.DB, n *model.Notification) error
	FindByUser(userID uint, unreadOnly bool, page, size int) ([]model.Notification, int64, error)
	UnreadCount(userID uint) int64
	MarkAsRead(userID, notifID uint) error
	MarkAllRead(userID uint) error
}

type notificationRepo struct{ db *gorm.DB }

func NewNotificationRepo(db *gorm.DB) NotificationRepo { return &notificationRepo{db: db} }

func (r *notificationRepo) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *notificationRepo) Create(tx *gorm.DB, n *model.Notification) error {
	return r.getDB(tx).Create(n).Error
}

func (r *notificationRepo) FindByUser(userID uint, unreadOnly bool, page, size int) ([]model.Notification, int64, error) {
	var notifs []model.Notification
	var total int64
	offset := (page - 1) * size

	q := r.db.Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("is_read = ?", false)
	}
	q.Model(&model.Notification{}).Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset(offset).Find(&notifs).Error
	return notifs, total, err
}

func (r *notificationRepo) UnreadCount(userID uint) int64 {
	var count int64
	r.db.Model(&model.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&count)
	return count
}

func (r *notificationRepo) MarkAsRead(userID, notifID uint) error {
	return r.db.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", notifID, userID).
		Update("is_read", true).Error
}

func (r *notificationRepo) MarkAllRead(userID uint) error {
	return r.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error
}
