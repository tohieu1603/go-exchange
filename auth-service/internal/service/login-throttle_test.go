package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newThrottle(t *testing.T) (*LoginThrottle, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewLoginThrottle(rdb), mr
}

func TestLoginThrottle_NoLockoutInitially(t *testing.T) {
	tr, _ := newThrottle(t)
	assert.Zero(t, tr.CheckLocked(context.Background(), "alice@example.com", "1.2.3.4"))
}

func TestLoginThrottle_PerEmailLockoutAfterMaxFailures(t *testing.T) {
	tr, _ := newThrottle(t)
	ctx := context.Background()

	// Email-side threshold = 5. The 5th failure must engage the lockout.
	for i := 1; i <= loginMaxFailedTries; i++ {
		count, rem := tr.RecordFailure(ctx, "alice@example.com", "1.2.3.4")
		assert.Equal(t, i, count)
		expectedRem := loginMaxFailedTries - i
		if expectedRem < 0 {
			expectedRem = 0
		}
		assert.Equal(t, expectedRem, rem)
	}
	assert.Greater(t, tr.CheckLocked(ctx, "alice@example.com", ""), time.Duration(0),
		"lockout should be active after threshold")
}

func TestLoginThrottle_PerIPLockoutHasHigherThreshold(t *testing.T) {
	// IP threshold (20) is intentionally higher than email (5) to avoid
	// punishing entire NAT'd offices for one mistyped login.
	tr, _ := newThrottle(t)
	ctx := context.Background()

	// Burn through different emails from the same IP — each email stops at 1
	// failure but the IP counter accumulates.
	for i := 0; i < loginMaxFailedIPTries; i++ {
		email := "user" + string(rune('a'+i)) + "@example.com"
		tr.RecordFailure(ctx, email, "5.6.7.8")
	}
	// IP lock fires only after the broader threshold.
	assert.Greater(t, tr.CheckLocked(ctx, "fresh@example.com", "5.6.7.8"), time.Duration(0),
		"IP lockout should engage after IP threshold even for a fresh email")
}

func TestLoginThrottle_RecordSuccessClearsEmailCounter(t *testing.T) {
	tr, _ := newThrottle(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		tr.RecordFailure(ctx, "bob@example.com", "1.2.3.4")
	}
	tr.RecordSuccess(ctx, "bob@example.com")

	// Next failure should restart at 1, not 5.
	count, _ := tr.RecordFailure(ctx, "bob@example.com", "1.2.3.4")
	assert.Equal(t, 1, count, "RecordSuccess should reset email-side counter")
}

func TestLoginThrottle_AdminUnlockClearsLockout(t *testing.T) {
	tr, _ := newThrottle(t)
	ctx := context.Background()
	for i := 0; i < loginMaxFailedTries; i++ {
		tr.RecordFailure(ctx, "carol@example.com", "1.2.3.4")
	}
	require.Greater(t, tr.CheckLocked(ctx, "carol@example.com", ""), time.Duration(0))

	tr.AdminUnlock(ctx, "carol@example.com")
	assert.Zero(t, tr.CheckLocked(ctx, "carol@example.com", ""),
		"AdminUnlock should fully clear the email-side lockout")
}

func TestLoginThrottle_NilSafe(t *testing.T) {
	// Defensive: helpers must not panic if throttle is nil or rdb is nil.
	// Code paths that disable throttling (tests, dev) rely on this.
	var tr *LoginThrottle
	assert.Zero(t, tr.CheckLocked(context.Background(), "x@y", "1.1.1.1"))

	tr = &LoginThrottle{rdb: nil}
	count, rem := tr.RecordFailure(context.Background(), "x@y", "1.1.1.1")
	assert.Equal(t, 0, count)
	assert.Equal(t, loginMaxFailedTries, rem)
}

func TestLowerEmail_Cases(t *testing.T) {
	cases := map[string]string{
		"ALICE@Example.COM": "alice@example.com",
		"bob@example.com":   "bob@example.com",
		"":                  "",
		"MIXED-Case+Tag@x":  "mixed-case+tag@x", // non-alpha chars pass through
	}
	for in, want := range cases {
		assert.Equal(t, want, lowerEmail(in))
	}
}
