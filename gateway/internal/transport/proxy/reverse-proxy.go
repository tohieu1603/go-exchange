package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// NewReverseProxy returns a Gin handler that proxies requests to target.
func NewReverseProxy(target string) gin.HandlerFunc {
	targetURL, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Preserve original Host header for downstream routing
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}

	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// ProxyGroup registers a catch-all route for a path prefix on the given engine.
func ProxyGroup(r *gin.Engine, pathPrefix, target string) {
	p := NewReverseProxy(target)
	r.Any(pathPrefix+"/*path", p)
}
