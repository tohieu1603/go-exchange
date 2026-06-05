package resilience

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeClock is an injectable, advanceable clock for deterministic timing tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestBreaker(maxFail int, openFor time.Duration) (*Breaker, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	b := New(Config{MaxFailures: maxFail, OpenFor: openFor, now: clk.now})
	return b, clk
}

var errBoom = errors.New("boom")

func TestNew_AppliesDefaults(t *testing.T) {
	b := New(Config{})
	assert.Equal(t, 5, b.cfg.MaxFailures)
	assert.Equal(t, 10*time.Second, b.cfg.OpenFor)
	assert.Equal(t, 1, b.cfg.HalfOpenSuccesses)
	assert.Equal(t, StateClosed, b.State())
}

func TestBreaker_TripsOpenAfterConsecutiveFailures(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	for i := 0; i < 2; i++ {
		assert.Equal(t, errBoom, b.Do(func() error { return errBoom }))
		assert.Equal(t, StateClosed, b.State(), "still closed below threshold")
	}
	assert.Equal(t, errBoom, b.Do(func() error { return errBoom }))
	assert.Equal(t, StateOpen, b.State(), "third failure trips open")

	// While open, fn is not invoked and ErrOpen is returned.
	called := false
	err := b.Do(func() error { called = true; return nil })
	assert.ErrorIs(t, err, ErrOpen)
	assert.False(t, called, "fn must not run while open")
}

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	_ = b.Do(func() error { return errBoom })
	_ = b.Do(func() error { return errBoom })
	_ = b.Do(func() error { return nil }) // resets streak
	_ = b.Do(func() error { return errBoom })
	assert.Equal(t, StateClosed, b.State(), "streak reset means we are below threshold again")
}

func TestBreaker_HalfOpenRecoversToClosedOnSuccess(t *testing.T) {
	b, clk := newTestBreaker(1, 30*time.Second)
	_ = b.Do(func() error { return errBoom }) // open
	assert.Equal(t, StateOpen, b.State())

	clk.advance(30 * time.Second) // window elapses -> half-open
	assert.Equal(t, StateHalfOpen, b.State())

	assert.NoError(t, b.Do(func() error { return nil })) // trial succeeds
	assert.Equal(t, StateClosed, b.State(), "successful trial closes the breaker")
}

func TestBreaker_HalfOpenTrialFailureReopens(t *testing.T) {
	b, clk := newTestBreaker(1, 30*time.Second)
	_ = b.Do(func() error { return errBoom })
	clk.advance(30 * time.Second)
	assert.Equal(t, StateHalfOpen, b.State())

	assert.Equal(t, errBoom, b.Do(func() error { return errBoom }))
	assert.Equal(t, StateOpen, b.State(), "failed trial re-opens immediately")
}

func TestBreaker_HalfOpenAllowsSingleTrialOnly(t *testing.T) {
	b, clk := newTestBreaker(1, 30*time.Second)
	_ = b.Do(func() error { return errBoom })
	clk.advance(30 * time.Second)

	// First Allow takes the single trial slot; the second is rejected until the
	// trial reports back.
	assert.True(t, b.Allow(), "first trial allowed")
	assert.False(t, b.Allow(), "second concurrent trial rejected")
}

func TestBreaker_ConcurrentUseIsRaceFree(t *testing.T) {
	b, _ := newTestBreaker(5, time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = b.Do(func() error {
				if n%2 == 0 {
					return errBoom
				}
				return nil
			})
			_ = b.State()
		}(i)
	}
	wg.Wait() // -race makes this meaningful
}
