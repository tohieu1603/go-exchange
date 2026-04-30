package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryptox/shared/utils"
	"github.com/gin-gonic/gin"
)

const testSecret = "test-secret-do-not-use"

func init() { gin.SetMode(gin.TestMode) }

func TestJWTAuth_RejectsMissingToken(t *testing.T) {
	r := gin.New()
	r.GET("/p", JWTAuth(testSecret), func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestJWTAuth_RejectsInvalidToken(t *testing.T) {
	r := gin.New()
	r.GET("/p", JWTAuth(testSecret), func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestJWTAuth_AcceptsBearerToken(t *testing.T) {
	tok, _ := utils.GenerateAccessToken(7, "alice@example.com", "USER", testSecret)
	r := gin.New()
	r.GET("/p", JWTAuth(testSecret), func(c *gin.Context) {
		// Verify claim propagation into the request scope.
		uid, _ := c.Get("userId")
		c.JSON(200, gin.H{"uid": uid})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_AcceptsCookie(t *testing.T) {
	// Cookie path is the primary auth flow for the browser frontend.
	tok, _ := utils.GenerateAccessToken(99, "bob@example.com", "USER", testSecret)
	r := gin.New()
	r.GET("/p", JWTAuth(testSecret), func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: tok})
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("cookie auth: want 200 got %d", w.Code)
	}
}

func TestAdminOnly_AllowsAdmin(t *testing.T) {
	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set("role", "ADMIN")
		c.Next()
	}, AdminOnly(), func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("admin: want 200 got %d", w.Code)
	}
}

func TestAdminOnly_RejectsUser(t *testing.T) {
	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set("role", "USER")
		c.Next()
	}, AdminOnly(), func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("non-admin: want 403 got %d", w.Code)
	}
}

func TestGetUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := GetUserID(c); got != 0 {
		t.Errorf("missing userId: want 0 got %d", got)
	}
	c.Set("userId", uint(42))
	if got := GetUserID(c); got != 42 {
		t.Errorf("present userId: want 42 got %d", got)
	}
	// Wrong type — must not panic, return 0.
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Set("userId", "not-a-uint")
	if got := GetUserID(c2); got != 0 {
		t.Errorf("wrong type: want 0 got %d", got)
	}
}
