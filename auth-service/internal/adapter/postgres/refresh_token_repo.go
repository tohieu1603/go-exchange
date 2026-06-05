package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshTokenRepo is the pgx+sqlc adapter for usecase.RefreshTokenRepo.
type RefreshTokenRepo struct{ q *sqlc.Queries }

func NewRefreshTokenRepo(pool *pgxpool.Pool) *RefreshTokenRepo {
	return &RefreshTokenRepo{q: sqlc.New(pool)}
}

var _ usecase.RefreshTokenRepo = (*RefreshTokenRepo)(nil)

func (r *RefreshTokenRepo) Create(ctx context.Context, rt *domain.RefreshToken) error {
	id, err := r.q.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		UserID: int64(rt.UserID), TokenHash: rt.TokenHash, FamilyID: rt.FamilyID,
		ParentID: uptrToI64(rt.ParentID), UserAgent: rt.UserAgent, Ip: rt.IP,
		IssuedAt: rt.IssuedAt, ExpiresAt: rt.ExpiresAt, RevokedReason: rt.RevokedReason,
	})
	if err != nil {
		return fmt.Errorf("postgres: create refresh token: %w", err)
	}
	rt.ID = uint(id)
	return nil
}

func (r *RefreshTokenRepo) FindByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	row, err := r.q.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get refresh token: %w", err)
	}
	return &domain.RefreshToken{
		ID: uint(row.ID), UserID: uint(row.UserID), TokenHash: row.TokenHash, FamilyID: row.FamilyID,
		ParentID: i64ptrToUint(row.ParentID), UserAgent: row.UserAgent, IP: row.Ip,
		IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt, UsedAt: row.UsedAt, RevokedAt: row.RevokedAt,
		RevokedReason: row.RevokedReason,
	}, nil
}

func (r *RefreshTokenRepo) MarkUsed(ctx context.Context, id uint) error {
	return wrap("mark refresh token used", r.q.MarkRefreshTokenUsed(ctx, int64(id)))
}

func (r *RefreshTokenRepo) RevokeFamily(ctx context.Context, familyID, reason string) error {
	return wrap("revoke refresh token family", r.q.RevokeRefreshTokenFamily(ctx, sqlc.RevokeRefreshTokenFamilyParams{FamilyID: familyID, RevokedReason: reason}))
}

func (r *RefreshTokenRepo) RevokeByUser(ctx context.Context, userID uint, reason string) error {
	return wrap("revoke refresh tokens by user", r.q.RevokeRefreshTokensByUser(ctx, sqlc.RevokeRefreshTokensByUserParams{UserID: int64(userID), RevokedReason: reason}))
}

func (r *RefreshTokenRepo) RevokeByID(ctx context.Context, id uint, reason string) error {
	return wrap("revoke refresh token by id", r.q.RevokeRefreshTokenByID(ctx, sqlc.RevokeRefreshTokenByIDParams{ID: int64(id), RevokedReason: reason}))
}

func (r *RefreshTokenRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteExpiredRefreshTokens(ctx, cutoff)
}
