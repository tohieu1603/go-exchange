package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogRepo is the pgx+sqlc adapter for usecase.AuditLogRepo.
type AuditLogRepo struct{ q *sqlc.Queries }

func NewAuditLogRepo(pool *pgxpool.Pool) *AuditLogRepo { return &AuditLogRepo{q: sqlc.New(pool)} }

var _ usecase.AuditLogRepo = (*AuditLogRepo)(nil)

func (r *AuditLogRepo) Create(ctx context.Context, l *domain.AuditLog) error {
	row, err := r.q.CreateAuditLog(ctx, sqlc.CreateAuditLogParams{
		UserID: int64(l.UserID), Email: l.Email, Action: l.Action, Outcome: l.Outcome,
		Ip: l.IP, UserAgent: l.UserAgent, DeviceID: l.DeviceID, NewDevice: l.NewDevice, Detail: l.Detail,
	})
	if err != nil {
		return fmt.Errorf("postgres: create audit log: %w", err)
	}
	l.ID = uint(row.ID)
	l.CreatedAt = row.CreatedAt
	return nil
}

func (r *AuditLogRepo) ListByUser(ctx context.Context, userID uint, page, size int) ([]domain.AuditLog, int64, error) {
	rows, err := r.q.ListAuditByUser(ctx, sqlc.ListAuditByUserParams{UserID: int64(userID), Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list audit by user: %w", err)
	}
	total, err := r.q.CountAuditByUser(ctx, int64(userID))
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count audit by user: %w", err)
	}
	return auditsToDomain(rows), total, nil
}

func (r *AuditLogRepo) ListAll(ctx context.Context, action string, page, size int) ([]domain.AuditLog, int64, error) {
	rows, err := r.q.ListAuditAll(ctx, sqlc.ListAuditAllParams{Action: action, Lim: int32(size), Off: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list audit all: %w", err)
	}
	total, err := r.q.CountAuditAll(ctx, action)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count audit all: %w", err)
	}
	return auditsToDomain(rows), total, nil
}

func (r *AuditLogRepo) PruneOlderThan(ctx context.Context, days int) int64 {
	if days <= 0 {
		return 0
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	n, err := r.q.PruneAuditOlderThan(ctx, cutoff)
	if err != nil {
		return 0
	}
	return n
}

func (r *AuditLogRepo) HasDeviceForUser(ctx context.Context, userID uint, deviceID string) bool {
	if userID == 0 || deviceID == "" {
		return true // treat as known to avoid spurious alerts
	}
	n, err := r.q.CountAuditDeviceForUser(ctx, sqlc.CountAuditDeviceForUserParams{UserID: int64(userID), DeviceID: deviceID})
	if err != nil {
		return true
	}
	return n > 0
}

func auditToDomain(m sqlc.AuditLog) domain.AuditLog {
	return domain.AuditLog{
		ID: uint(m.ID), UserID: uint(m.UserID), Email: m.Email, Action: m.Action, Outcome: m.Outcome,
		IP: m.Ip, UserAgent: m.UserAgent, DeviceID: m.DeviceID, NewDevice: m.NewDevice, Detail: m.Detail,
		CreatedAt: m.CreatedAt,
	}
}

func auditsToDomain(ms []sqlc.AuditLog) []domain.AuditLog {
	out := make([]domain.AuditLog, len(ms))
	for i, m := range ms {
		out[i] = auditToDomain(m)
	}
	return out
}
