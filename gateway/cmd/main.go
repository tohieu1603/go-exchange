package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authgrpc "github.com/cryptox/gateway/internal/grpc"
	"github.com/cryptox/shared/metrics"
	"github.com/cryptox/shared/tracing"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/utils"
	"github.com/cryptox/shared/ws"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

const swaggerAggregatorHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Micro-Exchange API Docs</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      dom_id: '#ui',
      urls: [
        {url: "http://localhost:8081/swagger/doc.json", name: "Auth Service (8081)"},
        {url: "http://localhost:8082/swagger/doc.json", name: "Wallet Service (8082)"},
        {url: "http://localhost:8083/swagger/doc.json", name: "Market Service (8083)"},
        {url: "http://localhost:8084/swagger/doc.json", name: "Trading Service (8084)"},
        {url: "http://localhost:8085/swagger/doc.json", name: "Futures Service (8085)"},
        {url: "http://localhost:8086/swagger/doc.json", name: "Notification Service (8086)"}
      ],
      "urls.primaryName": "Auth Service (8081)",
      layout: "BaseLayout"
    });
  </script>
</body>
</html>`

func swaggerAggregatorHandler(c *gin.Context) {
	c.Data(200, "text/html; charset=utf-8", []byte(swaggerAggregatorHTML))
}

func main() {
	_ = godotenv.Load() // load .env if present
	port := getEnv("PORT", "8080")
	jwtSecret := os.Getenv("JWT_SECRET")

	tracingShutdown := tracing.Init("gateway")
	defer tracingShutdown(context.Background())

	authURL := getEnv("AUTH_URL", "http://localhost:8081")
	walletURL := getEnv("WALLET_URL", "http://localhost:8082")
	marketURL := getEnv("MARKET_URL", "http://localhost:8083")
	tradingURL := getEnv("TRADING_URL", "http://localhost:8084")
	futuresURL := getEnv("FUTURES_URL", "http://localhost:8085")
	notifURL := getEnv("NOTIFICATION_URL", "http://localhost:8086")
	authGRPCAddr := getEnv("AUTH_GRPC_ADDR", "localhost:9081")

	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")
	opt, _ := redis.ParseURL(redisURL)
	rdb := redis.NewClient(opt)

	authClient := authgrpc.NewAuthClient(authGRPCAddr)

	hub := ws.NewHub(rdb, jwtSecret)
	go hub.Run()

	rl := redisutil.NewRateLimiter(rdb)

	r := gin.New()
	r.Use(
		gin.Logger(), gin.Recovery(),
		middleware.CORS(),
		otelgin.Middleware("gateway"), metrics.GinMiddleware("gateway"),
		// WAF runs before route resolution. Whitelist webhook endpoint because
		// SePay sends bank descriptions that may contain noise tripping rules.
		middleware.WAF("/api/webhook/sepay"),
	)
	r.GET("/metrics", metrics.Handler())

	// Swagger aggregator UI — no auth required
	r.GET("/swagger", swaggerAggregatorHandler)
	r.GET("/api/swagger", swaggerAggregatorHandler)

	// Health probe — exposed on both `/health` (standard) and `/api/health`
	// (matches the FE's NEXT_PUBLIC_API_URL prefix so a single base URL works).
	healthHandler := func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "ws_clients": hub.ClientCount()})
	}
	r.GET("/health", healthHandler)
	r.GET("/api/health", healthHandler)
	// WebSocket — exposed under both `/ws` (legacy) and `/api/ws` so the FE
	// can use a single `${NEXT_PUBLIC_API_URL}/ws` base. Either path works.
	r.GET("/ws", hub.HandleWS)
	r.GET("/api/ws", hub.HandleWS)

	// Route table: path prefix → target + auth required
	type route struct {
		target string
		auth   bool
	}
	routes := map[string]route{
		"/api/auth/register":  {authURL, false},
		"/api/auth/login":     {authURL, false},
		"/api/auth/2fa/login": {authURL, false},
		"/api/auth/refresh":   {authURL, false},
		"/api/webhook/":       {walletURL, false},
		"/api/market/":        {marketURL, false},
		"/api/auth/":          {authURL, true},
		"/api/kyc/":           {authURL, true},
		"/api/wallet/":        {walletURL, true},
		"/api/trading/":       {tradingURL, true},
		"/api/futures/":       {futuresURL, true},
		// No trailing slash: needs to match both the list path (`/api/notifications`)
		// and sub-resource paths (`/api/notifications/unread-count`, `/:id/read`).
		"/api/notifications": {notifURL, true},
		// Admin routes split by owning service
		"/api/admin/deposits":    {walletURL, true},
		"/api/admin/withdrawals": {walletURL, true},
		"/api/admin/":            {authURL, true},
	}

	// Single handler that matches path prefix and proxies
	// Global rate limit per client IP. 3000/min is generous for dev; matches a
	// trader heavy-clicking intervals/order book + background polling without
	// hitting 429. Tighter per-route limits live in the upstream services.
	r.NoRoute(rl.GinMiddleware("global", time.Minute, 3000), func(c *gin.Context) {
		path := c.Request.URL.Path

		// Find matching route (exact match first, then prefix)
		var matched route
		found := false

		// Special-cases: per-user admin sub-resources owned by service-specific tables.
		// Each sub-resource is short-circuited to its owning service rather than
		// the default `/api/admin/` → auth-service mapping.
		if strings.HasPrefix(path, "/api/admin/users/") {
			switch {
			case strings.HasSuffix(path, "/wallets"):
				matched = route{walletURL, true}
				found = true
			case strings.HasSuffix(path, "/orders"):
				matched = route{tradingURL, true}
				found = true
			case strings.HasSuffix(path, "/positions"):
				matched = route{futuresURL, true}
				found = true
			}
		}

		// Exact matches
		if !found {
			if r, ok := routes[path]; ok {
				matched = r
				found = true
			}
		}
		// Prefix matches — pick longest match
		if !found {
			bestLen := 0
			for prefix, r := range routes {
				if strings.HasPrefix(path, prefix) && len(prefix) > bestLen {
					matched = r
					bestLen = len(prefix)
					found = true
				}
			}
		}

		if !found {
			c.JSON(404, gin.H{"success": false, "message": "not found"})
			return
		}

		// Auth check — try API key (HMAC) first, then JWT cookie / header.
		// API keys are intended for algorithmic clients; cookies for browsers.
		if matched.auth {
			apiKeyID := c.GetHeader("X-API-Key")
			if apiKeyID != "" {
				// HMAC-signed request from a bot. Body is read once and rewound
				// before proxying (httputil.ReverseProxy reads it again).
				var bodyBytes []byte
				if c.Request.Body != nil {
					bodyBytes, _ = io.ReadAll(c.Request.Body)
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
				resp, err := authClient.ValidateAPIKey(c.Request.Context(),
					apiKeyID,
					c.GetHeader("X-API-Sign"),
					c.GetHeader("X-API-Timestamp"),
					c.Request.Method, c.Request.URL.Path,
					bodyBytes, c.ClientIP(),
				)
				if err != nil || !resp.Valid {
					msg := "Invalid API key"
					if resp != nil && resp.Error != "" {
						msg = resp.Error
					}
					c.JSON(401, gin.H{"success": false, "message": msg})
					return
				}
				// Mint a short-lived internal JWT so downstream services'
				// JWTAuth middleware accepts the request unchanged.
				internalJWT, jerr := utils.GenerateAccessToken(uint(resp.UserId), "", resp.Role, jwtSecret)
				if jerr != nil {
					c.JSON(500, gin.H{"success": false, "message": "internal auth error"})
					return
				}
				c.Request.Header.Set("Authorization", "Bearer "+internalJWT)
				c.Request.Header.Set("X-User-ID", fmt.Sprintf("%d", resp.UserId))
				c.Request.Header.Set("X-User-Role", resp.Role)
				c.Request.Header.Set("X-Auth-Method", "api_key")
				c.Request.Header.Set("X-API-Permissions", resp.Permissions)
			} else {
				// Cookie / Bearer JWT path.
				token := ""
				if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
					token = cookie
				} else {
					authHeader := c.GetHeader("Authorization")
					if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
						token = authHeader[7:]
					}
				}
				if token == "" {
					c.JSON(401, gin.H{"success": false, "message": "Token required"})
					return
				}
				resp, err := authClient.ValidateToken(c.Request.Context(), token)
				if err != nil || !resp.Valid {
					c.JSON(401, gin.H{"success": false, "message": "Invalid token"})
					return
				}
				c.Request.Header.Set("X-User-ID", fmt.Sprintf("%d", resp.UserId))
				c.Request.Header.Set("X-User-Email", resp.Email)
				c.Request.Header.Set("X-User-Role", resp.Role)
				c.Request.Header.Set("X-Auth-Method", "cookie")
			}
		}

		// Reverse proxy — strip CORS headers from backend (gateway owns CORS)
		target, _ := url.Parse(matched.target)
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Methods")
			resp.Header.Del("Access-Control-Allow-Headers")
			resp.Header.Del("Access-Control-Allow-Credentials")
			return nil
		}
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		log.Printf("[gateway] listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gateway failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[gateway] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
