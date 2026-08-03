package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
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
		if s.leaderGuard(w, r) {
			return true
		}
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

func (s *Server) leaderGuard(w http.ResponseWriter, r *http.Request) bool {
	if s.haLeader == nil || !s.haLeader.Enabled() {
		return false
	}
	if isReadOnlyExempt(r.Method, r.URL.Path) {
		return false
	}
	if s.haLeader.IsLeader(r.Context()) {
		return false
	}
	w.Header().Set("Retry-After", "30")
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":      "not cluster leader",
		"ha_enabled": true,
		"is_leader":  false,
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
	obCfg := storage.ObjectBackendFromEnv(s.cfg.DataDir, s.cfg.ReadOnly)
	resp["object_backend"] = obCfg.Backend
	if h := storage.HealthOf(r.Context(), s.backend); h.Degraded {
		resp["erasure_degraded"] = true
	} else if obCfg.Backend == "erasure" {
		resp["erasure_degraded"] = false
	}
	if s.haLeader != nil && s.haLeader.Enabled() {
		isLeader := s.haLeader.IsLeader(r.Context())
		resp["ha_enabled"] = true
		resp["is_leader"] = isLeader
		resp["node_id"] = s.haLeader.NodeID()
		observability.SetHALeaderMetrics(true, isLeader)
	} else {
		observability.SetHALeaderMetrics(false, false)
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
