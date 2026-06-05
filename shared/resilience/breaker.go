// Package resilience holds small, dependency-free fault-tolerance primitives
// used across services. The circuit breaker here protects callers of a flaky
// downstream (another service over gRPC, an external API) by failing fast once
// the dependency is clearly unhealthy, instead of piling thousands of doomed
// requests onto it and exhausting the caller's own goroutines/connections —
// the classic cascading-failure pattern.
package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned by Do/Call when the breaker is open (the downstream is
// considered unhealthy) and the request is rejected without being attempted.
var ErrOpen = errors.New("circuit breaker open")

// State is the breaker's current state.
type State int

const (
	// StateClosed: requests pass through; failures are counted.
	StateClosed State = iota
	// StateOpen: requests are rejected immediately with ErrOpen.
	StateOpen
	// StateHalfOpen: a single trial request is allowed to probe recovery.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// Config tunes a Breaker. The zero value is not used directly; New applies
// defaults for any non-positive field.
type Config struct {
	// MaxFailures is the number of consecutive failures that trips the breaker
	// from closed to open. Default 5.
	MaxFailures int
	// OpenFor is how long the breaker stays open before allowing a half-open
	// trial request. Default 10s.
	OpenFor time.Duration
	// HalfOpenSuccesses is the number of consecutive successes in half-open
	// required to close the breaker again. Default 1.
	HalfOpenSuccesses int
	// now is injectable for deterministic tests; nil means time.Now.
	now func() time.Time
}

// Breaker is a goroutine-safe circuit breaker.
type Breaker struct {
	cfg Config

	mu              sync.Mutex
	state           State
	consecFailures  int
	consecSuccesses int
	openedAt        time.Time
}

// New constructs a Breaker, applying defaults for any unset Config field.
func New(cfg Config) *Breaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.OpenFor <= 0 {
		cfg.OpenFor = 10 * time.Second
	}
	if cfg.HalfOpenSuccesses <= 0 {
		cfg.HalfOpenSuccesses = 1
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	return &Breaker{cfg: cfg, state: StateClosed}
}

// State returns the breaker's current state, advancing an expired open breaker
// to half-open as a side effect (so observers see the live state).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	return b.state
}

// Allow reports whether a request may proceed right now, reserving the single
// half-open trial slot when applicable. Callers that use Allow MUST report the
// outcome back via Success or Failure. Prefer Do, which couples the two.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	switch b.state {
	case StateOpen:
		return false
	case StateHalfOpen:
		// Allow exactly one trial: flip to open until the trial reports back so
		// concurrent callers don't all stampede the recovering downstream.
		b.state = StateOpen
		b.openedAt = b.cfg.now()
		return true
	default:
		return true
	}
}

// Success records a successful call, closing a half-open breaker once enough
// trials succeed and resetting the failure count.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecFailures = 0
	if b.state != StateClosed {
		b.consecSuccesses++
		if b.consecSuccesses >= b.cfg.HalfOpenSuccesses {
			b.state = StateClosed
			b.consecSuccesses = 0
		}
	}
}

// Failure records a failed call, tripping the breaker open once the failure
// threshold is reached (or immediately if a half-open trial fails).
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecSuccesses = 0
	b.consecFailures++
	if b.state == StateHalfOpen || b.consecFailures >= b.cfg.MaxFailures {
		b.state = StateOpen
		b.openedAt = b.cfg.now()
	}
}

// Do runs fn under the breaker: it returns ErrOpen without calling fn when the
// breaker is open, otherwise it calls fn and records the outcome. The error
// from fn is returned unchanged so callers keep the original cause.
func (b *Breaker) Do(fn func() error) error {
	if !b.Allow() {
		return ErrOpen
	}
	err := fn()
	if err != nil {
		b.Failure()
		return err
	}
	b.Success()
	return nil
}

// refresh transitions an open breaker whose OpenFor window has elapsed into
// half-open. Caller must hold b.mu.
func (b *Breaker) refresh() {
	if b.state == StateOpen && b.cfg.now().Sub(b.openedAt) >= b.cfg.OpenFor {
		b.state = StateHalfOpen
		b.consecSuccesses = 0
	}
}
