package response

import (
	"log"
	"net/http"

	"github.com/cryptox/shared/apperr"
	"github.com/cryptox/shared/grpcerr"
	"github.com/gin-gonic/gin"
)

// Internal records err server-side (for debugging) and replies with a generic
// 500 "internal error". Use it wherever a handler hits an unexpected failure so
// the underlying detail (DB strings, downstream addresses, stack hints) is
// logged for operators but never returned to the client.
func Internal(c *gin.Context, err error) {
	log.Printf("[500] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	Error(c, http.StatusInternalServerError, "internal error")
}

// Fail writes err to the client as the correct HTTP status with a safe message.
// It is the single place transport handlers render errors, guaranteeing that an
// internal/unclassified error never leaks its detail (DB strings, stack hints,
// downstream addresses) to the API — it always becomes 500 "internal error".
// Classified application errors (apperr.Invalid/NotFound/…) keep their
// caller-safe message and map to the matching 4xx status.
func Fail(c *gin.Context, err error) {
	Error(c, grpcerr.HTTPStatus(err), apperr.SafeMessage(err))
}

// FailClassified is Fail for services whose domain still returns plain sentinel
// errors rather than apperr values. When err carries an apperr Kind it is
// rendered exactly like Fail (Kind-derived status + safe message). Otherwise the
// supplied classify func — which encodes the service's sentinel→status
// knowledge — chooses the status, and the raw message is only exposed for
// client-error (4xx) statuses; any 5xx is reduced to "internal error" so a
// system failure never reaches the caller.
func FailClassified(c *gin.Context, err error, classify func(error) int) {
	if e, ok := apperr.As(err); ok {
		Error(c, grpcerr.HTTPStatus(e), apperr.SafeMessage(e))
		return
	}
	status := classify(err)
	msg := err.Error()
	if status >= 500 {
		msg = "internal error"
	}
	Error(c, status, msg)
}
