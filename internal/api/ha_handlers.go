package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

func readOnlyFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_READ_ONLY")))
	return v == "1" || v == "true" || v == "yes"
}

func (s *Server) isReadOnly() bool {
	return s.cfg.ReadOnly
}

func (s *Server) readOnlyGuard(w http.ResponseWriter, r *http.Request) bool {
	if !s.isReadOnly() {
		return false
	}
	if isReadOnlyExempt(r.Method, r.URL.Path) {
		return false
	}
	w.Header().Set("Retry-After", "300")
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":          "read-only mode",
		"read_only_mode": true,
	})
	return true
}

func isReadOnlyExempt(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return true
	}
	if path == "/healthz" || path == "/metrics" || strings.HasPrefix(path, "/api/v1/health") {
		return true
	}
	return false
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":         "ok",
		"read_only_mode": s.isReadOnly(),
	}
	if lag, ok := s.meta.ReplicationLagSeconds(); ok {
		resp["postgres_ok"] = true
		resp["replication_lag_s"] = lag
		observability.SetPostgresReplicationLag(lag)
	} else if s.cfg.Metadata.Backend == metadata.BackendPostgres {
		resp["postgres_ok"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}
