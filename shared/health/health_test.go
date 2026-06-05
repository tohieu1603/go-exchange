package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func mountAndServe(t *testing.T, r *Registry, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	eng := gin.New()
	r.Mount(eng)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	eng.ServeHTTP(w, req)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

func TestLive_AlwaysOK(t *testing.T) {
	r := New("wallet").Register("db", func(context.Context) error { return errors.New("down") })
	w, body := mountAndServe(t, r, "/healthz")
	assert.Equal(t, http.StatusOK, w.Code, "liveness ignores dependency health")
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "wallet", body["service"])
}

func TestReady_AllChecksPass(t *testing.T) {
	r := New("wallet").
		Register("db", func(context.Context) error { return nil }).
		Register("redis", func(context.Context) error { return nil })
	w, body := mountAndServe(t, r, "/readyz")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", body["status"])
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "ok", checks["db"])
	assert.Equal(t, "ok", checks["redis"])
}

func TestReady_OneCheckFails_Returns503AndHidesDetail(t *testing.T) {
	r := New("wallet").
		Register("db", func(context.Context) error { return nil }).
		Register("redis", func(context.Context) error { return errors.New("pq: secret connection string leaked") })
	w, body := mountAndServe(t, r, "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "degraded", body["status"])
	checks := body["checks"].(map[string]any)
	assert.Equal(t, "ok", checks["db"])
	assert.Equal(t, "fail", checks["redis"])
	assert.NotContains(t, w.Body.String(), "secret", "raw check error must not leak to the response")
}

func TestReady_NoChecks_IsHealthy(t *testing.T) {
	w, body := mountAndServe(t, New("gateway"), "/readyz")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", body["status"])
}

func TestReady_CheckReceivesDeadline(t *testing.T) {
	var hadDeadline bool
	r := New("svc").WithTimeout(50*time.Millisecond).Register("slow", func(ctx context.Context) error {
		_, hadDeadline = ctx.Deadline()
		return nil
	})
	w, _ := mountAndServe(t, r, "/readyz")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, hadDeadline, "each check must run under the per-check timeout")
}

func TestRegister_NilCheckIgnored(t *testing.T) {
	r := New("svc").Register("noop", nil)
	w, _ := mountAndServe(t, r, "/readyz")
	assert.Equal(t, http.StatusOK, w.Code)
}
