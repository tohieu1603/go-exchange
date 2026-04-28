package service

import (
	"context"
	"log"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/eventbus"
)

// AuditLogService is a fire-and-forget logger for security-sensitive events.
// All record errors are logged but never propagated — auditing must not block
// business flows.
//
// Persists rows to PostgreSQL (auth-service own DB) AND publishes
// audit.logged events for es-indexer → Elasticsearch → Kibana dashboards.
type AuditLogService struct {
	repo repository.AuditLogRepo
	bus  eventbus.EventPublisher
}

func NewAuditLogService(repo repository.AuditLogRepo) *AuditLogService {
	return &AuditLogService{repo: repo}
}

// SetBus wires the publisher post-construction so the audit service can be
// created before the bus is fully configured (avoids cyclic init order).
func (s *AuditLogService) SetBus(bus eventbus.EventPublisher) { s.bus = bus }

// Record persists a single audit row. Always non-blocking on caller's failure path.
func (s *AuditLogService) Record(userID uint, email, action, outcome, ip, ua, detail string) {
	s.RecordWithDevice(userID, email, action, outcome, ip, ua, "", detail)
}

// RecordWithDevice persists an audit row including device fingerprint. Looks up
// whether the (user, device) tuple has been seen before — sets NewDevice=true
// for first-time appearances so consumers can alert.
//
// Also publishes audit.logged event so es-indexer indexes the row in
// Elasticsearch for Kibana dashboards. Both writes are best-effort.
func (s *AuditLogService) RecordWithDevice(userID uint, email, action, outcome, ip, ua, deviceID, detail string) {
	if s == nil || s.repo == nil {
		return
	}
	newDevice := false
	if userID > 0 && deviceID != "" {
		newDevice = !s.repo.HasDeviceForUser(userID, deviceID)
	}
	row := &model.AuditLog{
		UserID: userID, Email: email,
		Action: action, Outcome: outcome,
		IP: ip, UserAgent: ua,
		DeviceID: deviceID, NewDevice: newDevice,
		Detail: detail,
	}
	go func() {
		if err := s.repo.Create(row); err != nil {
			log.Printf("[audit] persist failed action=%s user=%d: %v", action, userID, err)
		}
		// Publish to bus so es-indexer can mirror to Elasticsearch.
		if s.bus != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.bus.Publish(ctx, eventbus.TopicAuditLogged, eventbus.AuditLogEvent{
				UserID: userID, Email: email,
				Action: action, Outcome: outcome,
				IP: ip, UserAgent: ua,
				DeviceID: deviceID, NewDevice: newDevice,
				Detail:    detail,
				Timestamp: time.Now().UnixMilli(),
			})
		}
	}()
}

// Success / Failure are convenience shortcuts.
func (s *AuditLogService) Success(userID uint, email, action, ip, ua, detail string) {
	s.Record(userID, email, action, model.AuditOutcomeSuccess, ip, ua, detail)
}

// SuccessDevice records a successful action with device-fingerprint awareness.
func (s *AuditLogService) SuccessDevice(userID uint, email, action, ip, ua, deviceID, detail string) {
	s.RecordWithDevice(userID, email, action, model.AuditOutcomeSuccess, ip, ua, deviceID, detail)
}

func (s *AuditLogService) Failure(userID uint, email, action, ip, ua, detail string) {
	s.Record(userID, email, action, model.AuditOutcomeFailure, ip, ua, detail)
}

// IsNewDeviceForUser returns true if (userID, deviceID) was never seen before.
// Synchronous helper for callers that need the result inline (e.g. send email
// alert before returning login response).
func (s *AuditLogService) IsNewDeviceForUser(userID uint, deviceID string) bool {
	if s == nil || s.repo == nil || userID == 0 || deviceID == "" {
		return false
	}
	return !s.repo.HasDeviceForUser(userID, deviceID)
}

// ListByUser returns the user's own audit history (for `GET /api/auth/audit`).
func (s *AuditLogService) ListByUser(userID uint, page, size int) ([]model.AuditLog, int64, error) {
	return s.repo.ListByUser(userID, page, size)
}

// ListAll is admin-only — full platform audit query.
func (s *AuditLogService) ListAll(action string, page, size int) ([]model.AuditLog, int64, error) {
	return s.repo.ListAll(action, page, size)
}

// Prune deletes audit rows older than `retentionDays`. Called periodically
// from cmd/main.go. Returns rows deleted (for logging).
func (s *AuditLogService) Prune(retentionDays int) int64 {
	if s == nil || s.repo == nil {
		return 0
	}
	return s.repo.PruneOlderThan(retentionDays)
}
