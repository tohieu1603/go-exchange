package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// APIKeyRepo is the pgx+sqlc adapter for usecase.APIKeyRepo.
type APIKeyRepo struct{ q *sqlc.Queries }

func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{q: sqlc.New(pool)} }

var _ usecase.APIKeyRepo = (*APIKeyRepo)(nil)

func (r *APIKeyRepo) Create(ctx context.Context, k *domain.APIKey) error {
	row, err := r.q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		UserID: int64(k.UserID), Label: k.Label, KeyID: k.KeyID, SecretHash: k.SecretHash,
		Permissions: k.Permissions, IpWhitelist: k.IPWhitelist, ExpiresAt: k.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create api key: %w", err)
	}
	k.ID = uint(row.ID)
	k.CreatedAt = row.CreatedAt
	return nil
}

func (r *APIKeyRepo) FindByKeyID(ctx context.Context, keyID string) (*domain.APIKey, error) {
	row, err := r.q.GetAPIKeyByKeyID(ctx, keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get api key: %w", err)
	}
	k := apiKeyToDomain(row)
	return &k, nil
}

func (r *APIKeyRepo) ListByUser(ctx context.Context, userID uint) ([]domain.APIKey, error) {
	rows, err := r.q.ListAPIKeysByUser(ctx, int64(userID))
	if err != nil {
		return nil, fmt.Errorf("postgres: list api keys: %w", err)
	}
	out := make([]domain.APIKey, len(rows))
	for i, m := range rows {
		out[i] = apiKeyToDomain(m)
	}
	return out, nil
}

func (r *APIKeyRepo) Revoke(ctx context.Context, id, userID uint) error {
	return wrap("revoke api key", r.q.RevokeAPIKey(ctx, sqlc.RevokeAPIKeyParams{ID: int64(id), UserID: int64(userID)}))
}

func (r *APIKeyRepo) UpdateLastUsed(ctx context.Context, id uint, ip string) error {
	return wrap("update api key last used", r.q.UpdateAPIKeyLastUsed(ctx, sqlc.UpdateAPIKeyLastUsedParams{ID: int64(id), LastUsedIp: ip}))
}

func apiKeyToDomain(m sqlc.ApiKey) domain.APIKey {
	return domain.APIKey{
		ID: uint(m.ID), UserID: uint(m.UserID), Label: m.Label, KeyID: m.KeyID, SecretHash: m.SecretHash,
		Permissions: m.Permissions, IPWhitelist: m.IpWhitelist, LastUsedAt: m.LastUsedAt, LastUsedIP: m.LastUsedIp,
		ExpiresAt: m.ExpiresAt, RevokedAt: m.RevokedAt, CreatedAt: m.CreatedAt,
	}
}
