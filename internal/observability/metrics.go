package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_http_requests_total",
		Help: "Total HTTP requests by method, route class, and status code.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "datasafe_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	storageBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_storage_bytes",
		Help: "Total stored object bytes.",
	})

	bucketCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_buckets_total",
		Help: "Number of buckets.",
	})
)

type StorageStats func() (buckets int, bytes int64)

func MetricsHandler(stats StorageStats) http.Handler {
	base := promhttp.Handler()
	if stats == nil {
		return base
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetStorageStats(stats)
		base.ServeHTTP(w, r)
	})
}

func SetStorageStats(fn StorageStats) {
	if fn == nil {
		return
	}
	b, bytes := fn()
	bucketCount.Set(float64(b))
	storageBytes.Set(float64(bytes))
	refreshExtended()
	refreshHostMetrics()
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		route := routeClass(r.URL.Path)
		status := strconv.Itoa(ww.status)
		httpRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

func routeClass(path string) string {
	switch {
	case path == "/metrics":
		return "metrics"
	case path == "/healthz" || strings.HasPrefix(path, "/api/v1/health"):
		return "health"
	case strings.HasPrefix(path, "/api/v1/admin/login"):
		return "admin_login"
	case strings.HasPrefix(path, "/api/v1/"):
		return "admin_api"
	default:
		return "s3"
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
