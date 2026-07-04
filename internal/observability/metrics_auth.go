package observability

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

const metricsBearerPrefix = "Bearer "

// MetricsTokenFromEnv returns STORAGE_METRICS_TOKEN when set.
func MetricsTokenFromEnv() string {
	return strings.TrimSpace(os.Getenv("STORAGE_METRICS_TOKEN"))
}

// MetricsAuthHandler protects inner with bearer token auth when token is non-empty.
func MetricsAuthHandler(token string, inner http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" || inner == nil {
		return inner
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(got, metricsBearerPrefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(got, metricsBearerPrefix))
		if subtle.ConstantTimeCompare([]byte(raw), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}
