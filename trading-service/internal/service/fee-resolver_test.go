package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlatFeeResolver_ReturnsSameRatesForAnyUser(t *testing.T) {
	r := NewFlatFeeResolver(0.001, 0.002)
	for _, uid := range []uint{0, 1, 999, 1 << 30} {
		maker, taker := r.Rates(uid)
		assert.InDelta(t, 0.001, maker, 1e-9)
		assert.InDelta(t, 0.002, taker, 1e-9)
	}
}

func TestPlatformFeeUserID_DefaultZero(t *testing.T) {
	// In a fresh process the atomic is zero; callers should treat that as
	// "fee wallet not yet resolved" and skip crediting.
	platformFeeUserID.Store(0)
	assert.EqualValues(t, 0, PlatformFeeUserID())
}

func TestPlatformFeeUserID_AtomicReadAfterStore(t *testing.T) {
	defer platformFeeUserID.Store(0) // reset for other tests
	platformFeeUserID.Store(42)
	assert.EqualValues(t, 42, PlatformFeeUserID())
}

func TestPlatformFeeUserID_ConcurrentReadsSafe(t *testing.T) {
	defer platformFeeUserID.Store(0)
	platformFeeUserID.Store(7)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// No data race — atomic.Uint64 is the whole point.
			assert.EqualValues(t, 7, PlatformFeeUserID())
		}()
	}
	wg.Wait()
}
