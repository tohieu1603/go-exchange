// Package metrics centralizes Prometheus instrumentation. Each service registers
// the same standard counters/histograms via auto-init, plus optional domain
// metrics (login, orders, futures, anomaly) when relevant.
//
// Usage in main.go:
//
//	import "github.com/cryptox/shared/metrics"
//	r := gin.New()
//	r.Use(metrics.GinMiddleware("auth"))   // labels http_* with service=auth
//	r.GET("/metrics", metrics.Handler())   // expose /metrics
//
// Domain metrics are exported as package-level vars; callers do
//
//	metrics.LoginTotal.WithLabelValues("success").Inc()
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTP — generic per-service request metrics.
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests handled, labelled by service/method/path/status.",
	}, []string{"service", "method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by service/method/path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "path"})
)

// Domain metrics — visible to all services so any can increment if relevant.
// (Auth service writes login_total; trading writes orders_placed_total; etc.)
var (
	LoginTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "login_total",
		Help: "Login attempts by outcome (success / failure).",
	}, []string{"outcome"})

	OrdersPlaced = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "orders_placed_total",
		Help: "Spot orders submitted, labelled by side and type.",
	}, []string{"pair", "side", "type"})

	TradeVolumeUSDT = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trade_volume_usdt",
		Help: "Cumulative trade volume in USDT (price * amount on every fill).",
	})

	OrderBookSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "orderbook_size",
		Help: "Number of resting LIMIT orders on the in-memory book per pair.",
	}, []string{"pair"})

	FuturesOpenPositions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "futures_open_positions",
		Help: "Number of OPEN futures positions across all users.",
	})

	AnomalyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "anomaly_total",
		Help: "Anomaly detector firings, labelled by detector kind.",
	}, []string{"kind"})
)

// GinMiddleware records request count + duration for every handler. Path label
// uses the route template (`/api/users/:id`) — never the raw URL — so high-
// cardinality IDs don't blow up Prometheus.
func GinMiddleware(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		httpRequests.WithLabelValues(service, c.Request.Method, path, status).Inc()
		httpDuration.WithLabelValues(service, c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

// Handler returns a Gin handler exposing /metrics in the Prometheus text format.
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return gin.WrapH(h)
}
