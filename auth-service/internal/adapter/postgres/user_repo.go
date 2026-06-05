// Package postgres implements the auth persistence ports on top of pgx + sqlc.
// It imports the domain (entities) and the generated sqlc package only. The
// auth use cases never open transactions, so there is no TxManager here; each
// repo holds a *sqlc.Queries bound to the pool.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cryptox/auth-service/internal/adapter/postgres/sqlc"
	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepo is the pgx+sqlc adapter for usecase.UserRepo.
type UserRepo struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool, q: sqlc.New(pool)} }

var _ usecase.UserRepo = (*UserRepo)(nil)

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	row, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{
		Email: u.Email, PasswordHash: u.PasswordHash, FullName: u.FullName, Phone: u.Phone,
		KycStatus: defaultStr(u.KYCStatus, "NONE"), Is2Fa: u.Is2FA, TwoFaSecret: u.TwoFASecret,
		Role: defaultStr(u.Role, "USER"), EmailVerified: u.EmailVerified, KycStep: int32(u.KYCStep),
		IsLocked: u.IsLocked, LockReason: u.LockReason, LastLoginIp: u.LastLoginIP,
		RegisterIp: u.RegisterIP, GoogleSub: u.GoogleSub, AvatarUrl: u.AvatarURL,
	})
	if err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	*u = *userToDomain(row)
	return nil
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	return mapUserResult(row, err)
}

func (r *UserRepo) FindByID(ctx context.Context, id uint) (*domain.User, error) {
	row, err := r.q.GetUserByID(ctx, int64(id))
	return mapUserResult(row, err)
}

func (r *UserRepo) FindByGoogleSub(ctx context.Context, sub string) (*domain.User, error) {
	row, err := r.q.GetUserByGoogleSub(ctx, sub)
	return mapUserResult(row, err)
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	if err := r.q.UpdateUser(ctx, sqlc.UpdateUserParams{
		ID: int64(u.ID), Email: u.Email, PasswordHash: u.PasswordHash, FullName: u.FullName,
		Phone: u.Phone, KycStatus: u.KYCStatus, Is2Fa: u.Is2FA, TwoFaSecret: u.TwoFASecret,
		Role: u.Role, EmailVerified: u.EmailVerified, KycStep: int32(u.KYCStep), IsLocked: u.IsLocked,
		LockReason: u.LockReason, LastLoginIp: u.LastLoginIP, RegisterIp: u.RegisterIP,
		GoogleSub: u.GoogleSub, AvatarUrl: u.AvatarURL,
	}); err != nil {
		return fmt.Errorf("postgres: update user: %w", err)
	}
	return nil
}

// allowedUserCols whitelists the columns the dynamic UpdateField(s) may touch,
// guarding against SQL injection via attacker-influenced field names.
var allowedUserCols = map[string]bool{
	"password_hash": true, "last_login_ip": true, "register_ip": true, "two_fa_secret": true,
	"is2_fa": true, "kyc_step": true, "kyc_status": true, "email_verified": true,
	"full_name": true, "phone": true, "avatar_url": true, "is_locked": true,
	"lock_reason": true, "role": true, "google_sub": true,
}

func (r *UserRepo) UpdateField(ctx context.Context, id uint, field string, value interface{}) error {
	return r.UpdateFields(ctx, id, map[string]interface{}{field: value})
}

// UpdateFields applies a dynamic partial update. sqlc cannot express variable
// column sets, so this is hand-built pgx with a column whitelist.
func (r *UserRepo) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	cols := make([]string, 0, len(updates))
	args := make([]interface{}, 0, len(updates)+1)
	i := 1
	for col, val := range updates {
		if !allowedUserCols[col] {
			return fmt.Errorf("postgres: update users: column %q not allowed", col)
		}
		cols = append(cols, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}
	args = append(args, int64(id))
	query := fmt.Sprintf("UPDATE users SET %s, updated_at = now() WHERE id = $%d", strings.Join(cols, ", "), i)
	if _, err := r.pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("postgres: update users fields: %w", err)
	}
	return nil
}

func (r *UserRepo) Count(ctx context.Context) int64 {
	n, err := r.q.CountRealUsers(ctx)
	if err != nil {
		return 0
	}
	return n
}

func (r *UserRepo) FindPaginated(ctx context.Context, search string, page, size int) ([]domain.User, int64, error) {
	rows, err := r.q.ListUsersExclSystem(ctx, sqlc.ListUsersExclSystemParams{Search: search, Lim: int32(size), Off: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list users: %w", err)
	}
	total, err := r.q.CountUsersExclSystem(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count users: %w", err)
	}
	return usersToDomain(rows), total, nil
}

func (r *UserRepo) UpdateKYC(ctx context.Context, id uint, status string) error {
	if err := r.q.UpdateUserKYCStatus(ctx, sqlc.UpdateUserKYCStatusParams{ID: int64(id), KycStatus: status}); err != nil {
		return fmt.Errorf("postgres: update kyc: %w", err)
	}
	return nil
}

func (r *UserRepo) ListAdmin(ctx context.Context, search string, page, size int) ([]domain.User, int64, error) {
	rows, err := r.q.ListUsersAdmin(ctx, sqlc.ListUsersAdminParams{Search: search, Lim: int32(size), Off: int32((page - 1) * size)})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list users admin: %w", err)
	}
	total, err := r.q.CountUsersAdmin(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: count users admin: %w", err)
	}
	return usersToDomain(rows), total, nil
}

func (r *UserRepo) CountRealUsers(ctx context.Context) (int64, error) {
	return r.q.CountRealUsers(ctx)
}

func (r *UserRepo) CountByKYCStatus(ctx context.Context, status string) (int64, error) {
	return r.q.CountUsersByKYCStatus(ctx, status)
}

func (r *UserRepo) UserGrowthDaily(ctx context.Context, since time.Time) ([]usecase.DailyCount, error) {
	rows, err := r.q.UserGrowthDaily(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("postgres: user growth: %w", err)
	}
	out := make([]usecase.DailyCount, len(rows))
	for i, m := range rows {
		out[i] = usecase.DailyCount{Date: m.Day, Count: m.Count}
	}
	return out, nil
}

func mapUserResult(row sqlc.User, err error) (*domain.User, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get user: %w", err)
	}
	return userToDomain(row), nil
}

func userToDomain(m sqlc.User) *domain.User {
	return &domain.User{
		ID: uint(m.ID), Email: m.Email, PasswordHash: m.PasswordHash, FullName: m.FullName,
		Phone: m.Phone, KYCStatus: m.KycStatus, Is2FA: m.Is2Fa, TwoFASecret: m.TwoFaSecret,
		Role: m.Role, EmailVerified: m.EmailVerified, KYCStep: int(m.KycStep), IsLocked: m.IsLocked,
		LockReason: m.LockReason, LastLoginIP: m.LastLoginIp, RegisterIP: m.RegisterIp,
		GoogleSub: m.GoogleSub, AvatarURL: m.AvatarUrl, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		HasPassword: m.PasswordHash != "",
	}
}

func usersToDomain(ms []sqlc.User) []domain.User {
	out := make([]domain.User, len(ms))
	for i, m := range ms {
		out[i] = *userToDomain(m)
	}
	return out
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
