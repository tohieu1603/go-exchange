package repository

import (
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"gorm.io/gorm"
)

type AuditLogRepo interface {
	Create(log *model.AuditLog) error
	ListByUser(userID uint, page, size int) ([]model.AuditLog, int64, error)
	ListAll(action string, page, size int) ([]model.AuditLog, int64, error)
	PruneOlderThan(days int) int64
	HasDeviceForUser(userID uint, deviceID string) bool
}

type auditLogRepo struct{ db *gorm.DB }

func NewAuditLogRepo(db *gorm.DB) AuditLogRepo { return &auditLogRepo{db: db} }

func (r *auditLogRepo) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditLogRepo) ListByUser(userID uint, page, size int) ([]model.AuditLog, int64, error) {
	var rows []model.AuditLog
	var total int64
	q := r.db.Model(&model.AuditLog{}).Where("user_id = ?", userID)
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error
	return rows, total, err
}

func (r *auditLogRepo) ListAll(action string, page, size int) ([]model.AuditLog, int64, error) {
	var rows []model.AuditLog
	var total int64
	q := r.db.Model(&model.AuditLog{})
	if action != "" {
		q = q.Where("action = ?", action)
	}
	q.Count(&total)
	err := q.Order("created_at DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error
	return rows, total, err
}

// PruneOlderThan deletes audit rows older than `days` days. Returns rows deleted.
// Caller is expected to run this periodically (e.g. once a day).
func (r *auditLogRepo) PruneOlderThan(days int) int64 {
	if days <= 0 {
		return 0
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	res := r.db.Where("created_at < ?", cutoff).Delete(&model.AuditLog{})
	return res.RowsAffected
}

// HasDeviceForUser reports whether the (userID, deviceID) pair has been seen
// before in any prior audit row. Used to mark login.success entries as
// "new device" so security alerts can be triggered.
func (r *auditLogRepo) HasDeviceForUser(userID uint, deviceID string) bool {
	if userID == 0 || deviceID == "" {
		return true // treat as known to avoid spurious alerts
	}
	var count int64
	r.db.Model(&model.AuditLog{}).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Limit(1).Count(&count)
	return count > 0
}
