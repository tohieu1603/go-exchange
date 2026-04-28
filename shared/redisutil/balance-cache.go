package redisutil

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// BalanceCache provides Redis-backed atomic balance operations using Lua scripts.
// Hot path for trading - DB is settled async.
type BalanceCache struct {
	rdb *redis.Client
}

func NewBalanceCache(rdb *redis.Client) *BalanceCache {
	return &BalanceCache{rdb: rdb}
}

func balKey(userID uint, currency string) string {
	return fmt.Sprintf("bal:%d:%s", userID, currency)
}

func lockedKey(userID uint, currency string) string {
	return fmt.Sprintf("locked:%d:%s", userID, currency)
}

// LoadFromDB loads a wallet balance into Redis cache (overwrites).
func (bc *BalanceCache) LoadFromDB(ctx context.Context, userID uint, currency string, balance, locked float64) {
	bc.rdb.Set(ctx, balKey(userID, currency), fmt.Sprintf("%.10f", balance), 0)
	bc.rdb.Set(ctx, lockedKey(userID, currency), fmt.Sprintf("%.10f", locked), 0)
}

// WarmIfMissing only writes Redis when no value is cached. Use at cold-start
// so we don't clobber live state if Redis stayed up across a service restart.
func (bc *BalanceCache) WarmIfMissing(ctx context.Context, userID uint, currency string, balance, locked float64) {
	bk := balKey(userID, currency)
	lk := lockedKey(userID, currency)
	if _, err := bc.rdb.Get(ctx, bk).Result(); err != nil {
		bc.rdb.Set(ctx, bk, fmt.Sprintf("%.10f", balance), 0)
	}
	if _, err := bc.rdb.Get(ctx, lk).Result(); err != nil {
		bc.rdb.Set(ctx, lk, fmt.Sprintf("%.10f", locked), 0)
	}
}

// GetBalance returns cached balance (or 0, false if not cached).
func (bc *BalanceCache) GetBalance(ctx context.Context, userID uint, currency string) (float64, bool) {
	val, err := bc.rdb.Get(ctx, balKey(userID, currency)).Result()
	if err != nil {
		return 0, false
	}
	f, _ := strconv.ParseFloat(val, 64)
	return f, true
}

// GetLocked returns cached locked balance.
func (bc *BalanceCache) GetLocked(ctx context.Context, userID uint, currency string) float64 {
	val, err := bc.rdb.Get(ctx, lockedKey(userID, currency)).Result()
	if err != nil {
		return 0
	}
	f, _ := strconv.ParseFloat(val, 64)
	return f
}

// Deduct atomically checks and deducts balance. Returns new balance or error.
func (bc *BalanceCache) Deduct(ctx context.Context, userID uint, currency string, amount float64) (float64, error) {
	result, err := BalanceDeduct.Run(ctx, bc.rdb,
		[]string{balKey(userID, currency)},
		fmt.Sprintf("%.10f", amount),
	).Text()
	if err != nil {
		return 0, fmt.Errorf("lua deduct error: %w", err)
	}

	val, _ := strconv.ParseFloat(result, 64)
	if val == -2 {
		return 0, fmt.Errorf("wallet not cached")
	}
	if val == -1 {
		return 0, fmt.Errorf("insufficient balance")
	}
	return val, nil
}

// Credit atomically adds to balance.
func (bc *BalanceCache) Credit(ctx context.Context, userID uint, currency string, amount float64) (float64, error) {
	result, err := BalanceCredit.Run(ctx, bc.rdb,
		[]string{balKey(userID, currency)},
		fmt.Sprintf("%.10f", amount),
	).Text()
	if err != nil {
		return 0, fmt.Errorf("lua credit error: %w", err)
	}
	val, _ := strconv.ParseFloat(result, 64)
	return val, nil
}

// Lock atomically checks available and increases locked amount.
func (bc *BalanceCache) Lock(ctx context.Context, userID uint, currency string, amount float64) error {
	result, err := BalanceLock.Run(ctx, bc.rdb,
		[]string{balKey(userID, currency), lockedKey(userID, currency)},
		fmt.Sprintf("%.10f", amount),
	).Int64()
	if err != nil {
		return fmt.Errorf("lua lock error: %w", err)
	}
	if result == -1 {
		return fmt.Errorf("insufficient balance")
	}
	return nil
}

// Unlock atomically releases locked amount.
func (bc *BalanceCache) Unlock(ctx context.Context, userID uint, currency string, amount float64) error {
	_, err := BalanceUnlock.Run(ctx, bc.rdb,
		[]string{lockedKey(userID, currency)},
		fmt.Sprintf("%.10f", amount),
	).Text()
	return err
}

// Transfer atomically moves balance from one wallet to another.
func (bc *BalanceCache) Transfer(ctx context.Context, fromUser uint, fromCurrency string, toUser uint, toCurrency string, amount float64) error {
	result, err := BalanceTransfer.Run(ctx, bc.rdb,
		[]string{balKey(fromUser, fromCurrency), balKey(toUser, toCurrency)},
		fmt.Sprintf("%.10f", amount),
	).Int64()
	if err != nil {
		return fmt.Errorf("lua transfer error: %w", err)
	}
	if result == -1 {
		return fmt.Errorf("insufficient balance")
	}
	return nil
}
