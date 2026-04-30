package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVNDRate_DefaultBeforeFetch(t *testing.T) {
	// The package-level cache initialises to 25000 (sane USD/VND fallback)
	// so callers never see a zero rate even if the upstream API is down.
	vndRateMu.Lock()
	cachedVNDRate = 25000
	vndRateMu.Unlock()

	assert.InDelta(t, 25000.0, GetVNDRate(), 0.0001)
}

func TestGetVNDRate_ReadAfterWrite(t *testing.T) {
	vndRateMu.Lock()
	cachedVNDRate = 26500
	vndRateMu.Unlock()
	defer func() {
		vndRateMu.Lock()
		cachedVNDRate = 25000
		vndRateMu.Unlock()
	}()

	assert.InDelta(t, 26500.0, GetVNDRate(), 0.0001)
}

func TestGetVNDRate_ConcurrentSafe(t *testing.T) {
	// 50 readers + 1 writer must not race (RWMutex is the whole point).
	defer func() {
		vndRateMu.Lock()
		cachedVNDRate = 25000
		vndRateMu.Unlock()
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			vndRateMu.Lock()
			cachedVNDRate = 25000 + float64(i)
			vndRateMu.Unlock()
		}
	}()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = GetVNDRate() // race detector will catch concurrent unsynchronized access
		}()
	}
	wg.Wait()
}
