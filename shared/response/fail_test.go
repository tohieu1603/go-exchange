package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryptox/shared/apperr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func render(t *testing.T, fn func(*gin.Context)) (*httptest.ResponseRecorder, Response) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	fn(c)
	var body Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

func TestFail_ClassifiedKindsKeepSafeMessage(t *testing.T) {
	cases := []struct {
		err    error
		status int
		msg    string
	}{
		{apperr.Invalid("bad amount"), http.StatusBadRequest, "bad amount"},
		{apperr.NotFound("wallet not found"), http.StatusNotFound, "wallet not found"},
		{apperr.Conflict("already exists"), http.StatusConflict, "already exists"},
		{apperr.Unauthenticated("login required"), http.StatusUnauthorized, "login required"},
		{apperr.Forbidden("admins only"), http.StatusForbidden, "admins only"},
		{apperr.RateLimited("slow down"), http.StatusTooManyRequests, "slow down"},
	}
	for _, tc := range cases {
		w, body := render(t, func(c *gin.Context) { Fail(c, tc.err) })
		assert.Equal(t, tc.status, w.Code)
		assert.False(t, body.Success)
		assert.Equal(t, tc.msg, body.Message)
	}
}

func TestFail_InternalErrorIsHidden(t *testing.T) {
	leaky := apperr.Internal(errors.New("pq: password=hunter2 host=10.0.0.5"))
	w, body := render(t, func(c *gin.Context) { Fail(c, leaky) })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "internal error", body.Message)
	assert.NotContains(t, w.Body.String(), "hunter2", "secret must not leak")
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
}

func TestFail_PlainErrorTreatedAsInternal(t *testing.T) {
	w, body := render(t, func(c *gin.Context) { Fail(c, errors.New("raw driver error: connection reset")) })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "internal error", body.Message)
	assert.NotContains(t, w.Body.String(), "connection reset")
}

func TestFailClassified_SentinelMappedTo4xxKeepsMessage(t *testing.T) {
	sentinel := errors.New("insufficient balance")
	classify := func(error) int { return http.StatusBadRequest }
	w, body := render(t, func(c *gin.Context) { FailClassified(c, sentinel, classify) })
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "insufficient balance", body.Message, "domain message is safe and useful at 4xx")
}

func TestFailClassified_5xxMessageHidden(t *testing.T) {
	infra := errors.New("dial tcp 10.0.0.9:5432: connect: connection refused")
	classify := func(error) int { return http.StatusInternalServerError }
	w, body := render(t, func(c *gin.Context) { FailClassified(c, infra, classify) })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "internal error", body.Message)
	assert.NotContains(t, w.Body.String(), "10.0.0.9")
}

func TestFailClassified_ApperrTakesPrecedenceOverClassifier(t *testing.T) {
	// Even if the classifier would say 400, an apperr.Internal must be hidden.
	classify := func(error) int { return http.StatusBadRequest }
	w, body := render(t, func(c *gin.Context) { FailClassified(c, apperr.Internal(errors.New("boom")), classify) })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "internal error", body.Message)
}
