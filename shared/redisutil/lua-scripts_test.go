package redisutil

import (
	"context"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisForLua(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

// asFloat coerces a Lua return (which can be int or string) to float for
// numeric assertions.
func asFloat(t *testing.T, v interface{}) float64 {
	t.Helper()
	switch x := v.(type) {
	case int64:
		return float64(x)
	case string:
		f, err := strconv.ParseFloat(x, 64)
		require.NoError(t, err)
		return f
	}
	t.Fatalf("unexpected type %T (%v)", v, v)
	return 0
}

func TestBalanceCredit_FromZeroAndAdd(t *testing.T) {
	rdb, mr := newRedisForLua(t)
	ctx := context.Background()

	// Key absent → script SETs the initial value.
	out, err := BalanceCredit.Run(ctx, rdb, []string{"bal:1:USDT"}, 100).Result()
	require.NoError(t, err)
	assert.InDelta(t, 100.0, asFloat(t, out), 0.0001)

	// Subsequent credit accumulates.
	out, err = BalanceCredit.Run(ctx, rdb, []string{"bal:1:USDT"}, 50).Result()
	require.NoError(t, err)
	assert.InDelta(t, 150.0, asFloat(t, out), 0.0001)

	// Stored as the formatted decimal string.
	got, _ := mr.Get("bal:1:USDT")
	assert.Contains(t, got, "150")
}

func TestBalanceDeduct_HappyPath(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	// Seed.
	_, err := BalanceCredit.Run(ctx, rdb, []string{"bal:2:USDT"}, 100).Result()
	require.NoError(t, err)

	out, err := BalanceDeduct.Run(ctx, rdb, []string{"bal:2:USDT"}, 30).Result()
	require.NoError(t, err)
	assert.InDelta(t, 70.0, asFloat(t, out), 0.0001)
}

func TestBalanceDeduct_InsufficientReturnsMinus1(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	_, err := BalanceCredit.Run(ctx, rdb, []string{"bal:3:USDT"}, 10).Result()
	require.NoError(t, err)

	out, err := BalanceDeduct.Run(ctx, rdb, []string{"bal:3:USDT"}, 50).Result()
	require.NoError(t, err)
	assert.EqualValues(t, -1, out, "insufficient balance must return -1")
}

func TestBalanceDeduct_KeyMissingReturnsMinus2(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	out, err := BalanceDeduct.Run(context.Background(), rdb, []string{"bal:never"}, 10).Result()
	require.NoError(t, err)
	assert.EqualValues(t, -2, out, "missing key must return -2")
}

func TestBalanceLock_AvailableMinusLocked(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	// available = 100, locked = 30. Lock 50 → 30+50=80 ≤ 100 ⇒ OK.
	_, err := BalanceCredit.Run(ctx, rdb, []string{"bal:4:USDT"}, 100).Result()
	require.NoError(t, err)
	rdb.Set(ctx, "locked:4:USDT", "30", 0)

	out, err := BalanceLock.Run(ctx, rdb,
		[]string{"bal:4:USDT", "locked:4:USDT"}, 50).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, out, "lock within available should succeed")

	// Lock 30 more → available 100 - 80 (locked) = 20 < 30 ⇒ denied.
	out, err = BalanceLock.Run(ctx, rdb,
		[]string{"bal:4:USDT", "locked:4:USDT"}, 30).Result()
	require.NoError(t, err)
	assert.EqualValues(t, -1, out, "exceeding available should be denied")
}

func TestBalanceUnlock_NeverGoesNegative(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	rdb.Set(ctx, "locked:5:USDT", "10", 0)

	// Unlock more than is locked — script clamps at 0 to avoid negative.
	out, err := BalanceUnlock.Run(ctx, rdb, []string{"locked:5:USDT"}, 50).Result()
	require.NoError(t, err)
	assert.InDelta(t, 0.0, asFloat(t, out), 0.0001)
}

func TestBalanceTransfer_AtomicMove(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	_, err := BalanceCredit.Run(ctx, rdb, []string{"src"}, 100).Result()
	require.NoError(t, err)
	_, err = BalanceCredit.Run(ctx, rdb, []string{"dst"}, 5).Result()
	require.NoError(t, err)

	out, err := BalanceTransfer.Run(ctx, rdb, []string{"src", "dst"}, 30).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, out)

	srcRaw, _ := rdb.Get(ctx, "src").Result()
	dstRaw, _ := rdb.Get(ctx, "dst").Result()
	srcF, _ := strconv.ParseFloat(srcRaw, 64)
	dstF, _ := strconv.ParseFloat(dstRaw, 64)
	assert.InDelta(t, 70.0, srcF, 0.0001)
	assert.InDelta(t, 35.0, dstF, 0.0001)
}

func TestBalanceTransfer_InsufficientLeavesStateUnchanged(t *testing.T) {
	rdb, _ := newRedisForLua(t)
	ctx := context.Background()
	_, err := BalanceCredit.Run(ctx, rdb, []string{"src"}, 10).Result()
	require.NoError(t, err)
	_, err = BalanceCredit.Run(ctx, rdb, []string{"dst"}, 1).Result()
	require.NoError(t, err)

	out, err := BalanceTransfer.Run(ctx, rdb, []string{"src", "dst"}, 50).Result()
	require.NoError(t, err)
	assert.EqualValues(t, -1, out)

	// Both balances must be untouched (atomic abort).
	srcRaw, _ := rdb.Get(ctx, "src").Result()
	dstRaw, _ := rdb.Get(ctx, "dst").Result()
	srcF, _ := strconv.ParseFloat(srcRaw, 64)
	dstF, _ := strconv.ParseFloat(dstRaw, 64)
	assert.InDelta(t, 10.0, srcF, 0.0001, "src untouched on failed transfer")
	assert.InDelta(t, 1.0, dstF, 0.0001, "dst untouched on failed transfer")
}
