// Package health provides liveness and readiness endpoints shared by every
// service. Liveness ("am I running?") lets an orchestrator restart a wedged
// process; readiness ("can I serve traffic?") lets a load balancer pull a node
// out of rotation while its dependencies (Postgres, Redis, …) are unavailable —
// the difference between a graceful degradation and a user-visible outage under
// load. Check failures are reported as a status only; the underlying error is
// logged server-side and never returned to the caller.
package health

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Checker probes one dependency. It returns nil when healthy. The context
// carries the per-check timeout.
type Checker func(ctx context.Context) error

type namedChecker struct {
	name  string
	check Checker
}

// Registry holds the readiness checks for a service and renders the endpoints.
// The zero value is not usable; construct with New.
type Registry struct {
	mu      sync.RWMutex
	checks  []namedChecker
	timeout time.Duration
	service string
}

// New creates a Registry for the named service. Each readiness probe runs every
// check with a default 2s timeout (override with WithTimeout).
func New(service string) *Registry {
	return &Registry{service: service, timeout: 2 * time.Second}
}

// WithTimeout sets the per-check timeout and returns the Registry for chaining.
func (r *Registry) WithTimeout(d time.Duration) *Registry {
	if d > 0 {
		r.timeout = d
	}
	return r
}

// Register adds a named readiness check and returns the Registry for chaining.
func (r *Registry) Register(name string, c Checker) *Registry {
	if c == nil {
		return r
	}
	r.mu.Lock()
	r.checks = append(r.checks, namedChecker{name: name, check: c})
	r.mu.Unlock()
	return r
}

// Mount registers GET /healthz (liveness) and GET /readyz (readiness) on rg.
func (r *Registry) Mount(rg gin.IRoutes) {
	rg.GET("/healthz", r.Live)
	rg.GET("/readyz", r.Ready)
}

// Live always reports the process is up. It performs no dependency checks so an
// orchestrator only restarts on a genuinely dead process, not a transient
// dependency blip.
func (r *Registry) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": r.service})
}

// Ready runs every registered check and returns 200 only if all pass, else 503.
// Per-check results are reported as "ok"/"fail"; the underlying error is logged
// but never exposed.
func (r *Registry) Ready(c *gin.Context) {
	r.mu.RLock()
	checks := make([]namedChecker, len(r.checks))
	copy(checks, r.checks)
	timeout := r.timeout
	r.mu.RUnlock()

	results := make(gin.H, len(checks))
	allOK := true
	for _, nc := range checks {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		err := nc.check(ctx)
		cancel()
		if err != nil {
			allOK = false
			results[nc.name] = "fail"
			log.Printf("[%s][health] readiness check %q failed: %v", r.service, nc.name, err)
			continue
		}
		results[nc.name] = "ok"
	}

	status := http.StatusOK
	overall := "ok"
	if !allOK {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}
	c.JSON(status, gin.H{"status": overall, "service": r.service, "checks": results})
}
