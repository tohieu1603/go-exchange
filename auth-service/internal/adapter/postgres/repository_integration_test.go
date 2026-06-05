//go:build integration

// Integration tests for the auth pgx+sqlc adapters against a real Postgres
// (Testcontainers). Run with:  go test -tags=integration ./...
// Covers the security-critical paths: user CRUD + dynamic field updates + the
// HasPassword derivation, refresh-token rotation/revocation/cleanup, API-key
// revoke filtering, referral-commission idempotency, the fraud trade-pair
// upsert, and platform-settings upsert.
package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cryptox/auth-service/internal/domain"
	"github.com/cryptox/shared/pgxdb"
	"github.com/cryptox/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("auth"), tcpostgres.WithUsername("auth"), tcpostgres.WithPassword("auth"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	var pool *pgxpool.Pool
	for i := 0; i < 20; i++ {
		if pool, err = pgxdb.NewPool(ctx, dsn, 4); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	dir, _ := filepath.Abs(filepath.Join("..", "..", "..", "db", "migrations"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return pool
}

func TestUserRepo_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepo(newTestPool(t))

	u := &domain.User{Email: "alice@example.com", PasswordHash: "hash", FullName: "Alice", Role: "USER"}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 || !u.HasPassword || u.KYCStatus != "NONE" || u.CreatedAt.IsZero() {
		t.Fatalf("Create must populate id/defaults/HasPassword: %+v", u)
	}

	// Reads + HasPassword derivation on read.
	got, err := repo.FindByEmail(ctx, "alice@example.com")
	if err != nil || got.ID != u.ID || !got.HasPassword {
		t.Fatalf("FindByEmail = %+v, %v", got, err)
	}
	if _, err := repo.FindByEmail(ctx, "nobody@example.com"); err != domain.ErrNotFound {
		t.Fatalf("missing email must be ErrNotFound, got %v", err)
	}

	// Dynamic field update + whitelist enforcement.
	if err := repo.UpdateField(ctx, u.ID, "kyc_status", "PENDING"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}
	if err := repo.UpdateFields(ctx, u.ID, map[string]interface{}{"is2_fa": true, "kyc_step": 2}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	if err := repo.UpdateField(ctx, u.ID, "id; DROP TABLE users", 1); err == nil {
		t.Fatalf("UpdateField must reject a non-whitelisted column")
	}
	got, _ = repo.FindByID(ctx, u.ID)
	if got.KYCStatus != "PENDING" || !got.Is2FA || got.KYCStep != 2 {
		t.Fatalf("dynamic updates not applied: %+v", got)
	}

	// Count excludes SYSTEM; search + admin list.
	sys := &domain.User{Email: "sys@x.io", PasswordHash: "", Role: "SYSTEM"}
	_ = repo.Create(ctx, sys)
	if n := repo.Count(ctx); n != 1 { // only alice counts (SYSTEM excluded)
		t.Fatalf("Count(excl SYSTEM) = %d, want 1", n)
	}
	list, total, err := repo.FindPaginated(ctx, "alice", 1, 10)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("FindPaginated(alice) = len%d total%d %v", len(list), total, err)
	}
	if _, totalAll, _ := repo.ListAdmin(ctx, "", 1, 10); totalAll != 2 { // admin sees SYSTEM too
		t.Fatalf("ListAdmin total = %d, want 2", totalAll)
	}
	if c, _ := repo.CountByKYCStatus(ctx, "PENDING"); c != 1 {
		t.Fatalf("CountByKYCStatus(PENDING) = %d, want 1", c)
	}
	if rows, err := repo.UserGrowthDaily(ctx, time.Now().AddDate(0, 0, -1)); err != nil || len(rows) == 0 {
		t.Fatalf("UserGrowthDaily = %+v, %v", rows, err)
	}
}

func TestRefreshTokenRepo_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewRefreshTokenRepo(newTestPool(t))

	rt := &domain.RefreshToken{UserID: 1, TokenHash: "h1", FamilyID: "fam1", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Create(ctx, rt); err != nil || rt.ID == 0 {
		t.Fatalf("Create: %v (id=%d)", err, rt.ID)
	}
	got, err := repo.FindByHash(ctx, "h1")
	if err != nil || got.UsedAt != nil || got.RevokedAt != nil || got.ParentID != nil {
		t.Fatalf("FindByHash = %+v, %v; optional fields should be nil", got, err)
	}
	if err := repo.MarkUsed(ctx, rt.ID); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if got, _ = repo.FindByHash(ctx, "h1"); got.UsedAt == nil {
		t.Fatalf("MarkUsed did not set used_at")
	}

	// A second token in the same family; RevokeFamily revokes both active ones.
	rt2 := &domain.RefreshToken{UserID: 1, TokenHash: "h2", FamilyID: "fam1", ParentID: &rt.ID, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	_ = repo.Create(ctx, rt2)
	if got2, _ := repo.FindByHash(ctx, "h2"); got2.ParentID == nil || *got2.ParentID != rt.ID {
		t.Fatalf("parent_id not persisted: %+v", got2)
	}
	if err := repo.RevokeFamily(ctx, "fam1", domain.RevokeReasonReplayDetected); err != nil {
		t.Fatalf("RevokeFamily: %v", err)
	}
	if got2, _ := repo.FindByHash(ctx, "h2"); got2.RevokedAt == nil || got2.RevokedReason != domain.RevokeReasonReplayDetected {
		t.Fatalf("RevokeFamily did not revoke h2: %+v", got2)
	}

	// DeleteExpired removes the long-revoked rows.
	n, err := repo.DeleteExpired(ctx, time.Now().Add(time.Minute))
	if err != nil || n == 0 {
		t.Fatalf("DeleteExpired = %d, %v; want >0", n, err)
	}
	if _, err := repo.FindByHash(ctx, "h1"); err != domain.ErrNotFound {
		t.Fatalf("expired token should be gone, got %v", err)
	}
}

func TestAPIKeyRepo_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewAPIKeyRepo(newTestPool(t))

	k := &domain.APIKey{UserID: 5, Label: "bot", KeyID: "pub123", SecretHash: "sh", Permissions: "read,trade"}
	if err := repo.Create(ctx, k); err != nil || k.ID == 0 {
		t.Fatalf("Create: %v", err)
	}
	if got, err := repo.FindByKeyID(ctx, "pub123"); err != nil || got.Permissions != "read,trade" {
		t.Fatalf("FindByKeyID = %+v, %v", got, err)
	}
	if err := repo.UpdateLastUsed(ctx, k.ID, "1.2.3.4"); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	if got, _ := repo.FindByKeyID(ctx, "pub123"); got.LastUsedAt == nil || got.LastUsedIP != "1.2.3.4" {
		t.Fatalf("UpdateLastUsed not applied: %+v", got)
	}
	// Revoke hides the key from the active lookup.
	if err := repo.Revoke(ctx, k.ID, 5); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := repo.FindByKeyID(ctx, "pub123"); err != domain.ErrNotFound {
		t.Fatalf("revoked key must not be found, got %v", err)
	}
	if keys, _ := repo.ListByUser(ctx, 5); len(keys) != 1 { // ListByUser still shows it
		t.Fatalf("ListByUser = %d, want 1", len(keys))
	}
}

func TestBonusReferralFraudSettings_Integration(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	// Bonus: sum of remaining active bonus.
	bonus := NewBonusRepo(pool)
	promo := &domain.BonusPromotion{Name: "Welcome", BonusPercent: 50, TargetType: "ALL", TriggerType: "MANUAL", IsActive: true, StartAt: time.Now(), EndAt: time.Now().Add(24 * time.Hour)}
	if err := bonus.CreatePromotion(ctx, promo); err != nil {
		t.Fatalf("CreatePromotion: %v", err)
	}
	ub := &domain.UserBonus{UserID: 9, PromotionID: promo.ID, BonusAmount: 100, UsedAmount: 30, Status: "ACTIVE"}
	if err := bonus.CreateUserBonus(ctx, ub); err != nil {
		t.Fatalf("CreateUserBonus: %v", err)
	}
	if sum, _ := bonus.SumActiveBonus(ctx, 9); sum != 70 {
		t.Fatalf("SumActiveBonus = %v, want 70", sum)
	}

	// Referral commission is idempotent on trade_id.
	ref := NewReferralRepo(pool)
	c := &domain.ReferralCommission{ReferrerID: 1, RefereeID: 2, TradeID: 555, Currency: "USDT", FeeAmount: 1, Rate: 0.2, Commission: 0.2}
	if err := ref.CreateCommission(ctx, c); err != nil {
		t.Fatalf("CreateCommission: %v", err)
	}
	first := c.ID
	dup := &domain.ReferralCommission{ReferrerID: 1, RefereeID: 2, TradeID: 555, Currency: "USDT", FeeAmount: 9, Rate: 9, Commission: 9}
	if err := ref.CreateCommission(ctx, dup); err != nil || dup.ID != first {
		t.Fatalf("idempotent CreateCommission should return id %d, got %d (%v)", first, dup.ID, err)
	}

	// Fraud trade-pair upsert increments.
	fraud := NewFraudRepo(pool)
	c1, err := fraud.UpsertTradePair(ctx, 1, 2, "BTC_USDT", 100)
	if err != nil || c1.TradeCount != 1 || c1.TotalVol != 100 {
		t.Fatalf("first upsert = %+v, %v", c1, err)
	}
	c2, _ := fraud.UpsertTradePair(ctx, 1, 2, "BTC_USDT", 50)
	if c2.TradeCount != 2 || c2.TotalVol != 150 {
		t.Fatalf("second upsert should increment: %+v", c2)
	}

	// Settings: not-found, then upsert + read.
	settings := NewSettingsRepo(pool)
	if _, err := settings.Get(ctx); err != domain.ErrNotFound {
		t.Fatalf("empty settings must be ErrNotFound, got %v", err)
	}
	want := &types.PlatformSettings{ID: 1, DepositFeePercent: 2.5, TradingFeePercent: 0.1, MinDeposit: 100, KYCRequired: true}
	if err := settings.Upsert(ctx, want); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := settings.Get(ctx)
	if err != nil || got.DepositFeePercent != 2.5 || !got.KYCRequired {
		t.Fatalf("settings Get = %+v, %v", got, err)
	}
}
