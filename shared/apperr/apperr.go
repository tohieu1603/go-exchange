// Package apperr is the application-wide error model shared by every service's
// domain and usecase layers (mirrors the appios platform/apperr convention). It
// is PURE — it imports nothing from infrastructure — so domain code can depend
// on it without violating the dependency rule.
//
// Errors carry a Kind that transport layers (grpcerr, httpx) map to the right
// gRPC code / HTTP status without leaking internal detail.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error so transport layers can map it to a status code.
type Kind int

const (
	// KindInternal is an unexpected server-side failure; detail is never leaked.
	KindInternal Kind = iota
	// KindInvalid is a bad request / validation failure.
	KindInvalid
	// KindUnauthenticated means the caller is not authenticated.
	KindUnauthenticated
	// KindForbidden means the caller is authenticated but not permitted.
	KindForbidden
	// KindNotFound means the requested resource does not exist.
	KindNotFound
	// KindConflict means the request conflicts with current state.
	KindConflict
	// KindRateLimited means the caller exceeded a rate limit.
	KindRateLimited
)

// String returns a stable lower-case label for the Kind.
func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindRateLimited:
		return "rate_limited"
	default:
		return "internal"
	}
}

// Error is the canonical application error: a transport-mappable Kind, a safe
// human-readable Msg, and an optional wrapped Err for errors.Is/As chaining.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	switch {
	case e.Msg != "" && e.Err != nil:
		return e.Msg + ": " + e.Err.Error()
	case e.Msg != "":
		return e.Msg
	case e.Err != nil:
		return e.Err.Error()
	default:
		return e.Kind.String()
	}
}

// Unwrap exposes the wrapped error for errors.Is / errors.As traversal.
func (e *Error) Unwrap() error { return e.Err }

// Invalid returns a KindInvalid error.
func Invalid(msg string) error { return &Error{Kind: KindInvalid, Msg: msg} }

// Invalidf returns a KindInvalid error with a formatted message.
func Invalidf(format string, a ...any) error {
	return &Error{Kind: KindInvalid, Msg: fmt.Sprintf(format, a...)}
}

// NotFound returns a KindNotFound error.
func NotFound(msg string) error { return &Error{Kind: KindNotFound, Msg: msg} }

// Conflict returns a KindConflict error.
func Conflict(msg string) error { return &Error{Kind: KindConflict, Msg: msg} }

// Unauthenticated returns a KindUnauthenticated error.
func Unauthenticated(msg string) error { return &Error{Kind: KindUnauthenticated, Msg: msg} }

// Forbidden returns a KindForbidden error.
func Forbidden(msg string) error { return &Error{Kind: KindForbidden, Msg: msg} }

// RateLimited returns a KindRateLimited error.
func RateLimited(msg string) error { return &Error{Kind: KindRateLimited, Msg: msg} }

// Internal wraps an arbitrary error as KindInternal; detail is kept for logging
// but never sent to clients (transport layers hide it).
func Internal(err error) error { return &Error{Kind: KindInternal, Err: err} }

// Wrap annotates err with a Kind and message, preserving the chain. Returns nil
// if err is nil.
func Wrap(kind Kind, err error, msg string) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Msg: msg, Err: err}
}

// KindOf extracts the Kind of err (KindInternal for nil/unclassified).
func KindOf(err error) Kind {
	if err == nil {
		return KindInternal
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// Is reports whether err carries the given Kind.
func Is(err error, kind Kind) bool { return KindOf(err) == kind }

// As extracts the underlying *Error from err's chain, reporting whether one was
// found. Unlike KindOf it distinguishes a genuine application error from an
// unclassified one, which transport layers use to decide whether a message is
// safe to expose or must be replaced with a generic one.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// SafeMessage returns a client-safe message for err: the application message
// for a classified error, or a generic "internal error" for an internal /
// unclassified error so server-side detail is never leaked to the caller.
func SafeMessage(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := As(err); ok && e.Kind != KindInternal {
		return e.Error()
	}
	return "internal error"
}
