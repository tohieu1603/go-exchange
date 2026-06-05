// Package httpx provides production-hardened HTTP server construction shared by
// every service. A bare &http.Server{Addr, Handler} has *no* timeouts, which
// leaves the process open to slow-loris and slow-body resource-exhaustion
// attacks and lets a single stuck handler pin a connection forever. NewServer
// applies sane bounds in one place so no service has to remember them, and
// RunGraceful gives every service identical, correct shutdown behaviour.
package httpx

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Timeouts holds the connection-level deadlines applied to every server. The
// zero value is never used directly; DefaultTimeouts supplies production values
// and callers may override individual fields before passing to NewServerWith.
type Timeouts struct {
	// ReadHeaderTimeout bounds how long the server waits for request headers;
	// the primary slow-loris defence. Must be > 0.
	ReadHeaderTimeout time.Duration
	// ReadTimeout bounds the whole request read (headers + body).
	ReadTimeout time.Duration
	// WriteTimeout bounds the whole response write. Kept generous so large
	// paginated reads / file responses are not truncated under load.
	WriteTimeout time.Duration
	// IdleTimeout bounds how long a keep-alive connection may stay idle.
	IdleTimeout time.Duration
	// MaxHeaderBytes caps request header size to reject header-bomb requests.
	MaxHeaderBytes int
}

// DefaultTimeouts returns the production defaults used when a service does not
// customise them.
func DefaultTimeouts() Timeouts {
	return Timeouts{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}
}

// NewServer builds an *http.Server bound to addr with DefaultTimeouts applied.
// addr is a listen address such as ":8082".
func NewServer(addr string, h http.Handler) *http.Server {
	return NewServerWith(addr, h, DefaultTimeouts())
}

// NewServerWith is NewServer with caller-supplied timeouts. Any zero field
// falls back to the corresponding DefaultTimeouts value so a partially-filled
// Timeouts is always safe.
func NewServerWith(addr string, h http.Handler, t Timeouts) *http.Server {
	d := DefaultTimeouts()
	if t.ReadHeaderTimeout <= 0 {
		t.ReadHeaderTimeout = d.ReadHeaderTimeout
	}
	if t.ReadTimeout <= 0 {
		t.ReadTimeout = d.ReadTimeout
	}
	if t.WriteTimeout <= 0 {
		t.WriteTimeout = d.WriteTimeout
	}
	if t.IdleTimeout <= 0 {
		t.IdleTimeout = d.IdleTimeout
	}
	if t.MaxHeaderBytes <= 0 {
		t.MaxHeaderBytes = d.MaxHeaderBytes
	}
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: t.ReadHeaderTimeout,
		ReadTimeout:       t.ReadTimeout,
		WriteTimeout:      t.WriteTimeout,
		IdleTimeout:       t.IdleTimeout,
		MaxHeaderBytes:    t.MaxHeaderBytes,
	}
}

// NewWSServer builds an *http.Server for a service that also serves long-lived
// connections (WebSocket / SSE). It deliberately leaves ReadTimeout and
// WriteTimeout unset (0 = no whole-request deadline) so a streaming connection
// is not killed mid-stream, while still keeping the slow-loris and resource
// bounds that are safe for streams: a header read deadline, an idle-connection
// cap, and a maximum header size. Per-message deadlines are the stream
// handler's responsibility (gorilla/websocket sets its own).
func NewWSServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}
}

// Shutdown gracefully drains srv, waiting up to timeout for in-flight requests
// before forcing closed. It is a thin wrapper that supplies the bounded context
// so each service does not re-implement it.
func Shutdown(srv *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// ListenAndServe starts srv and returns nil on a clean shutdown
// (http.ErrServerClosed), surfacing any other startup error to the caller. It
// exists so callers can treat "closed by Shutdown" as success without
// re-checking the sentinel everywhere.
func ListenAndServe(srv *http.Server) error {
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
