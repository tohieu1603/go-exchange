package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// Cookie names. In production with HTTPS, prefer __Host- prefix
// (browser-enforced: Secure + Path=/, no Domain attribute).
const (
	AccessCookieName  = "access_token"
	RefreshCookieName = "refresh_token"

	// RefreshCookiePath limits refresh cookie to auth endpoints — browser only
	// sends the token where it is needed (rotation, logout).
	RefreshCookiePath = "/api/auth"
)

// SetAuthCookies sets HttpOnly cookies for access + refresh tokens.
// - access: SameSite=Lax, Path=/  (lifetime 15m)
// - refresh: SameSite=Strict, Path=/api/auth  (lifetime 7d)
// Both HttpOnly + Secure (in production).
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string) {
	secure := os.Getenv("COOKIE_SECURE") == "true"
	domain := os.Getenv("COOKIE_DOMAIN")

	// Access cookie — Lax so navigations to the SPA carry it
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AccessCookieName, accessToken, 900, "/", domain, secure, true)

	if refreshToken != "" {
		// Refresh cookie — Strict, scoped to /api/auth only
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(RefreshCookieName, refreshToken, 7*24*3600, RefreshCookiePath, domain, secure, true)
	}
}

// SetAccessCookie writes only the access cookie (used after refresh rotation
// when the refresh token did NOT change — but with rotation, this is rare).
func SetAccessCookie(c *gin.Context, accessToken string) {
	secure := os.Getenv("COOKIE_SECURE") == "true"
	domain := os.Getenv("COOKIE_DOMAIN")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AccessCookieName, accessToken, 900, "/", domain, secure, true)
}

// ClearAuthCookies removes auth cookies on logout.
func ClearAuthCookies(c *gin.Context) {
	domain := os.Getenv("COOKIE_DOMAIN")
	secure := os.Getenv("COOKIE_SECURE") == "true"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AccessCookieName, "", -1, "/", domain, secure, true)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(RefreshCookieName, "", -1, RefreshCookiePath, domain, secure, true)
}
