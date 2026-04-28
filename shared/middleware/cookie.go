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

// cookieAttrs reads the deployment-shaped cookie envelope from env once per
// call. Values:
//
//	COOKIE_SECURE=true|false      → set Secure flag (mandatory for SameSite=None).
//	COOKIE_DOMAIN=.example.com    → enables cross-subdomain auth (app.example.com + api.example.com).
//	COOKIE_SAMESITE=lax|strict|none → defaults: access=Lax, refresh=Strict.
//	                                When the SPA and API are on different
//	                                registrable domains in production, set
//	                                this to `none` (and Secure=true) so the
//	                                browser sends the cookie on cross-site
//	                                XHR. Otherwise leave default.
func cookieAttrs() (secure bool, domain string, sameSite http.SameSite) {
	secure = os.Getenv("COOKIE_SECURE") == "true"
	domain = os.Getenv("COOKIE_DOMAIN")
	switch os.Getenv("COOKIE_SAMESITE") {
	case "none", "None", "NONE":
		sameSite = http.SameSiteNoneMode
	case "strict", "Strict", "STRICT":
		sameSite = http.SameSiteStrictMode
	default:
		sameSite = http.SameSiteLaxMode
	}
	return
}

// SetAuthCookies sets HttpOnly cookies for access + refresh tokens.
//
// Defaults (suitable for same-origin dev + same-site prod):
//   - access: SameSite=Lax, Path=/  (lifetime 15m)
//   - refresh: SameSite=Strict by default, scoped to /api/auth  (lifetime 7d)
//
// Override via COOKIE_SAMESITE when deploying SPA+API on different registrable
// domains (browsers enforce SameSite=None+Secure for cross-site cookies).
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string) {
	secure, domain, defaultSameSite := cookieAttrs()

	// Access cookie — Lax so navigations to the SPA carry it.
	// In cross-domain prod, COOKIE_SAMESITE=none flips this to None.
	c.SetSameSite(defaultSameSite)
	c.SetCookie(AccessCookieName, accessToken, 900, "/", domain, secure, true)

	if refreshToken != "" {
		// Refresh cookie — stricter by default; same env override applies.
		refreshSameSite := http.SameSiteStrictMode
		if defaultSameSite == http.SameSiteNoneMode {
			refreshSameSite = http.SameSiteNoneMode
		}
		c.SetSameSite(refreshSameSite)
		c.SetCookie(RefreshCookieName, refreshToken, 7*24*3600, RefreshCookiePath, domain, secure, true)
	}
}

// SetAccessCookie writes only the access cookie (used after refresh rotation
// when the refresh token did NOT change — but with rotation, this is rare).
func SetAccessCookie(c *gin.Context, accessToken string) {
	secure, domain, sameSite := cookieAttrs()
	c.SetSameSite(sameSite)
	c.SetCookie(AccessCookieName, accessToken, 900, "/", domain, secure, true)
}

// ClearAuthCookies removes auth cookies on logout. SameSite must mirror the
// SET path so the browser overwrites the existing cookie rather than creating
// a sibling that survives the delete.
func ClearAuthCookies(c *gin.Context) {
	secure, domain, sameSite := cookieAttrs()
	c.SetSameSite(sameSite)
	c.SetCookie(AccessCookieName, "", -1, "/", domain, secure, true)

	refreshSameSite := http.SameSiteStrictMode
	if sameSite == http.SameSiteNoneMode {
		refreshSameSite = http.SameSiteNoneMode
	}
	c.SetSameSite(refreshSameSite)
	c.SetCookie(RefreshCookieName, "", -1, RefreshCookiePath, domain, secure, true)
}
