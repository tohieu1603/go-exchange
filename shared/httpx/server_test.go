package httpx

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer_AppliesDefaultTimeouts(t *testing.T) {
	srv := NewServer(":0", http.NewServeMux())
	d := DefaultTimeouts()
	assert.Equal(t, d.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	assert.Equal(t, d.ReadTimeout, srv.ReadTimeout)
	assert.Equal(t, d.WriteTimeout, srv.WriteTimeout)
	assert.Equal(t, d.IdleTimeout, srv.IdleTimeout)
	assert.Equal(t, d.MaxHeaderBytes, srv.MaxHeaderBytes)
	assert.Equal(t, ":0", srv.Addr)
}

func TestNewServerWith_ZeroFieldsFallBackToDefaults(t *testing.T) {
	// Only override WriteTimeout; every other field is zero and must be filled.
	srv := NewServerWith(":0", http.NewServeMux(), Timeouts{WriteTimeout: 7 * time.Second})
	d := DefaultTimeouts()
	assert.Equal(t, 7*time.Second, srv.WriteTimeout, "explicit value kept")
	assert.Equal(t, d.ReadHeaderTimeout, srv.ReadHeaderTimeout, "zero filled from default")
	assert.Equal(t, d.IdleTimeout, srv.IdleTimeout)
	assert.Equal(t, d.MaxHeaderBytes, srv.MaxHeaderBytes)
}

func TestNewWSServer_LeavesStreamTimeoutsUnset(t *testing.T) {
	srv := NewWSServer(":0", http.NewServeMux())
	assert.Zero(t, srv.WriteTimeout, "WriteTimeout must stay 0 so streams aren't cut")
	assert.Zero(t, srv.ReadTimeout, "ReadTimeout must stay 0 so streams aren't cut")
	assert.Equal(t, 10*time.Second, srv.ReadHeaderTimeout, "header read still bounded (slow-loris)")
	assert.Equal(t, 120*time.Second, srv.IdleTimeout)
	assert.Equal(t, 1<<20, srv.MaxHeaderBytes)
}

func TestListenAndServe_TreatsServerClosedAsSuccess(t *testing.T) {
	srv := NewServer("127.0.0.1:0", http.NewServeMux())
	// Pre-close so ListenAndServe returns http.ErrServerClosed immediately.
	require.NoError(t, srv.Close())
	assert.NoError(t, ListenAndServe(srv), "ErrServerClosed must not surface as an error")
}

func TestShutdown_ReturnsWithinTimeout(t *testing.T) {
	srv := NewServer("127.0.0.1:0", http.NewServeMux())
	// Server never started; Shutdown should return promptly without blocking.
	start := time.Now()
	err := Shutdown(srv, 2*time.Second)
	assert.NoError(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestShutdown_ReturnsPromptlyWithExpiredContext(t *testing.T) {
	// With no open connections, Shutdown completes immediately regardless of the
	// context deadline (there is nothing to drain) — the point is that it never
	// hangs.
	srv := NewServer("127.0.0.1:0", http.NewServeMux())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	start := time.Now()
	_ = srv.Shutdown(ctx)
	assert.Less(t, time.Since(start), time.Second, "Shutdown must not hang")
}
