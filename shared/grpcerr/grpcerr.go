// Package grpcerr maps between apperr.Kind and gRPC status codes (and HTTP
// statuses for the REST edge). Server code converts domain errors to a *status
// via ToStatus; gateway/inter-service code converts back via FromStatus.
// Internal errors never leak their detail.
package grpcerr

import (
	"errors"
	"net/http"

	"github.com/cryptox/shared/apperr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ToStatus converts an application error into a gRPC status error. Internal
// (and unclassified) errors are hidden behind a generic message.
func ToStatus(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		if !isApp(err) {
			return err // already a gRPC status from a downstream call
		}
	}
	kind := apperr.KindOf(err)
	if kind == apperr.KindInternal {
		return status.Error(codes.Internal, "internal error")
	}
	return status.Error(codeForKind(kind), err.Error())
}

// FromStatus converts a gRPC status error back into an apperr error.
func FromStatus(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return apperr.Internal(err)
	}
	msg := st.Message()
	switch st.Code() {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return apperr.Invalid(msg)
	case codes.Unauthenticated:
		return apperr.Unauthenticated(msg)
	case codes.PermissionDenied:
		return apperr.Forbidden(msg)
	case codes.NotFound:
		return apperr.NotFound(msg)
	case codes.AlreadyExists, codes.Aborted:
		return apperr.Conflict(msg)
	case codes.ResourceExhausted:
		return apperr.RateLimited(msg)
	default:
		return apperr.Wrap(apperr.KindInternal, err, msg)
	}
}

// HTTPStatus maps an application error to an HTTP status code (used by the REST
// transport / gateway).
func HTTPStatus(err error) int {
	switch apperr.KindOf(err) {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func codeForKind(kind apperr.Kind) codes.Code {
	switch kind {
	case apperr.KindInvalid:
		return codes.InvalidArgument
	case apperr.KindUnauthenticated:
		return codes.Unauthenticated
	case apperr.KindForbidden:
		return codes.PermissionDenied
	case apperr.KindNotFound:
		return codes.NotFound
	case apperr.KindConflict:
		return codes.AlreadyExists
	case apperr.KindRateLimited:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}

func isApp(err error) bool {
	var e *apperr.Error
	return errors.As(err, &e)
}
