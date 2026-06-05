package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() { gin.SetMode(gin.TestMode) }

func TestRecovery_PanicBecomesSafe500(t *testing.T) {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/boom", func(c *gin.Context) { panic("secret internal state: token=abc123") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "internal error")
	assert.NotContains(t, w.Body.String(), "abc123", "panic detail must not leak to the client")
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "true")
}

func TestTimeout_InjectsDeadline(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(50 * time.Millisecond))
	var hadDeadline bool
	r.GET("/x", func(c *gin.Context) {
		_, hadDeadline = c.Request.Context().Deadline()
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.True(t, hadDeadline, "handler context must carry the request deadline")
}

func TestTimeout_ZeroIsNoOp(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(0))
	var hadDeadline bool
	r.GET("/x", func(c *gin.Context) {
		_, hadDeadline = c.Request.Context().Deadline()
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.False(t, hadDeadline, "zero duration disables the deadline")
}
