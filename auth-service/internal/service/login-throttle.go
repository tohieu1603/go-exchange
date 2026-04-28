package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// LoginThrottle implements TWO parallel rate-limit dimensions for login:
//
//   A. Per-account (email)   — defends against targeted account guessing
//   B. Per-IP                 — defends against distributed credential stuffing
//                                (one attacker, many emails, same IP)
//
// Redis keys (all keyed lowercase):
//   failed_login:{email}         counter, TTL = window
//   login_locked:{email}         lockout flag, TTL = lockoutPeriod
//   failed_login_ip:{ip}         counter, TTL = window
//   login_locked_ip:{ip}         lockout flag, TTL = lockoutPeriod
//
// Lookup hits either lockout → reject login WITHOUT running bcrypt/argon2id.
// IP threshold is higher than account threshold so legitimate small offices
// behind NAT aren't punished by one user's typos.
type LoginThrottle struct {
	rdb *redis.Client
}

func NewLoginThrottle(rdb *redis.Client) *LoginThrottle {
	return &LoginThrottle{rdb: rdb}
}

const (
	loginFailWindow       = 15 * time.Minute
	loginLockoutPeriod    = 15 * time.Minute
	loginMaxFailedTries   = 5  // per email
	loginMaxFailedIPTries = 20 // per IP — broader because shared/NAT IPs are common
)

func failKey(email string) string     { return "failed_login:" + lowerEmail(email) }
func lockKey(email string) string     { return "login_locked:" + lowerEmail(email) }
func failIPKey(ip string) string      { return "failed_login_ip:" + ip }
func lockIPKey(ip string) string      { return "login_locked_ip:" + ip }

// CheckLocked returns the remaining seconds of an active account OR IP
// lockout, whichever fires first. 0 means no lockout.
func (t *LoginThrottle) CheckLocked(ctx context.Context, email, ip string) time.Duration {
	if t == nil || t.rdb == nil {
		return 0
	}
	emailTTL, _ := t.rdb.TTL(ctx, lockKey(email)).Result()
	ipTTL := time.Duration(0)
	if ip != "" {
		ipTTL, _ = t.rdb.TTL(ctx, lockIPKey(ip)).Result()
	}
	switch {
	case emailTTL > 0 && ipTTL > 0:
		if emailTTL > ipTTL {
			return emailTTL
		}
		return ipTTL
	case emailTTL > 0:
		return emailTTL
	case ipTTL > 0:
		return ipTTL
	}
	return 0
}

// RecordFailure increments BOTH email and IP counters; sets the corresponding
// lockout flag(s) when thresholds are crossed.
//
// Returns the per-email count and remaining attempts (for UI hints). IP-side
// counters are tracked but not surfaced — clients shouldn't learn the IP threshold.
func (t *LoginThrottle) RecordFailure(ctx context.Context, email, ip string) (count int, remaining int) {
	if t == nil || t.rdb == nil {
		return 0, loginMaxFailedTries
	}
	pipe := t.rdb.TxPipeline()
	emailIncr := pipe.Incr(ctx, failKey(email))
	pipe.Expire(ctx, failKey(email), loginFailWindow)
	var ipIncr *redis.IntCmd
	if ip != "" {
		ipIncr = pipe.Incr(ctx, failIPKey(ip))
		pipe.Expire(ctx, failIPKey(ip), loginFailWindow)
	}
	_, _ = pipe.Exec(ctx)

	n, _ := strconv.Atoi(fmt.Sprintf("%d", emailIncr.Val()))
	if n >= loginMaxFailedTries {
		t.rdb.Set(ctx, lockKey(email), "1", loginLockoutPeriod)
	}
	if ipIncr != nil {
		ipN := int(ipIncr.Val())
		if ipN >= loginMaxFailedIPTries {
			t.rdb.Set(ctx, lockIPKey(ip), "1", loginLockoutPeriod)
		}
	}
	rem := loginMaxFailedTries - n
	if rem < 0 {
		rem = 0
	}
	return n, rem
}

// RecordSuccess clears the per-email counters. Per-IP counters are NOT cleared
// because a successful login from the IP doesn't prove other earlier attempts
// were legitimate.
func (t *LoginThrottle) RecordSuccess(ctx context.Context, email string) {
	if t == nil || t.rdb == nil {
		return
	}
	t.rdb.Del(ctx, failKey(email), lockKey(email))
}

// AdminUnlock manually clears all lockouts associated with an account.
func (t *LoginThrottle) AdminUnlock(ctx context.Context, email string) {
	if t == nil || t.rdb == nil {
		return
	}
	t.rdb.Del(ctx, failKey(email), lockKey(email))
}

// AdminUnlockIP clears IP-level lockout (operations / abuse-team tool).
func (t *LoginThrottle) AdminUnlockIP(ctx context.Context, ip string) {
	if t == nil || t.rdb == nil || ip == "" {
		return
	}
	t.rdb.Del(ctx, failIPKey(ip), lockIPKey(ip))
}

func lowerEmail(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
