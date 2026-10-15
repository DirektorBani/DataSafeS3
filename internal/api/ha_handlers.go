package api

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/buildinfo"
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
	if r.Header.Get("X-DataSafe-Forwarded") == "" && s.forwardToLeader(w, r) {
		return true
	}
	w.Header().Set("Retry-After", "30")
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":      "not cluster leader",
		"ha_enabled": true,
		"is_leader":  false,
	})
	return true
}

// forwardToLeader proxies a write to the lease holder and keeps the original
// Host so SigV4 still matches. A missing address or a failed dial leaves the
// caller to answer 503.
func (s *Server) forwardToLeader(w http.ResponseWriter, r *http.Request) bool {
	holder, ok, err := s.meta.LiveLeader()
	if err != nil || !ok || holder == "" || holder == s.haLeader.NodeID() {
		return false
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		return false
	}
	addr := ""
	if beats, err := s.meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute)); err == nil {
		for _, b := range beats {
			if b.NodeID == holder && strings.TrimSpace(b.Address) != "" {
				addr = strings.TrimSpace(b.Address)
				break
			}
		}
	}
	if addr == "" {
		for _, n := range cfg.Cluster.Nodes {
			if n.ID == holder {
				addr = strings.TrimSpace(n.Address)
				break
			}
		}
	}
	if addr == "" {
		return false
	}
	return proxyToLeader(w, r, addr)
}

func proxyToLeader(w http.ResponseWriter, r *http.Request, leaderBase string) bool {
	leaderBase = strings.TrimSpace(leaderBase)
	if leaderBase == "" {
		return false
	}
	if !strings.Contains(leaderBase, "://") {
		leaderBase = "http://" + leaderBase
	}
	target, err := url.Parse(strings.TrimRight(leaderBase, "/") + r.URL.RequestURI())
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		return false
	}
	req.Header = r.Header.Clone()
	req.Header.Set("X-DataSafe-Forwarded", "1")
	req.Host = r.Host
	req.ContentLength = r.ContentLength
	resp, err := leaderForwardClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
}

// leaderForwardClient dials the previous holder quickly and then leaves the
// body alone. A dead leader fails the dial inside 2s and leaderGuard answers
// HTTP 503. A large PUT is not cut off by a response timeout.
var leaderForwardClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2: true,
		MaxIdleConns:      20,
		IdleConnTimeout:   90 * time.Second,
	},
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
		"version":        s3.HeartbeatVersion(buildinfo.Version),
		"commit":         buildinfo.Commit,
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
	kmsProvider, kmsAvailable := s.kmsHealth()
	resp["kms"] = map[string]any{"provider": kmsProvider, "available": kmsAvailable}
	if lag, ok := s.meta.ReplicationLagSeconds(); ok {
		resp["postgres_ok"] = true
		resp["replication_lag_s"] = lag
		observability.SetPostgresReplicationLag(lag)
	} else if s.cfg.Metadata.Backend == metadata.BackendPostgres {
		resp["postgres_ok"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}
