package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "https://app.example.com,https://admin.example.com")
	r := gin.New()
	r.Use(CORS())
	r.GET("/p", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("ACAO: want exact origin echo, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("ACAC: want true, got %q", got)
	}
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "https://app.example.com")
	r := gin.New()
	r.Use(CORS())
	r.GET("/p", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(w, req)

	// Reflect-only model: unknown origins must NOT get an ACAO header.
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO leaked for unknown origin: %q", got)
	}
}

func TestCORS_PreflightReturns204(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "https://app.example.com")
	r := gin.New()
	r.Use(CORS())
	r.GET("/p", func(c *gin.Context) { c.String(200, "should not reach") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/p", nil)
	req.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Errorf("preflight: want 204, got %d", w.Code)
	}
	// Allow-Headers must include the API-key headers since the SDK sends them.
	allow := w.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Authorization", "X-API-Key", "X-API-Sign", "X-API-Timestamp"} {
		if !strings.Contains(allow, h) {
			t.Errorf("Allow-Headers missing %q (got %q)", h, allow)
		}
	}
}

func TestAllowedOrigins_DefaultDevValues(t *testing.T) {
	// Empty env → safe localhost defaults so dev doesn't have to set CORS_ORIGINS.
	t.Setenv("CORS_ORIGINS", "")
	got := allowedOrigins()
	if len(got) != 2 || got[0] != "http://localhost:3000" || got[1] != "http://localhost:3001" {
		t.Errorf("default origins drifted: %v", got)
	}
}
