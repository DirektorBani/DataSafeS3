package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

var errTrustedClusterRevoked = errors.New("trusted cluster revoked or inactive")

func trustedClusterWorkerInterval() time.Duration {
	return 30 * time.Second
}

type trustedClusterWorker struct {
	mu      sync.Mutex
	healthy map[string]bool
}

func (s *Server) runTrustedClusterWorker(ctx context.Context) {
	w := &trustedClusterWorker{healthy: make(map[string]bool)}
	s.trustedClusterW = w
	ticker := time.NewTicker(trustedClusterWorkerInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processTrustedClusterPeersOnce(ctx, w)
		}
	}
}

func (s *Server) processTrustedClusterPeersOnce(ctx context.Context, w *trustedClusterWorker) {
	if s.haLeader != nil && s.haLeader.Enabled() && !s.haLeader.IsLeader(ctx) {
		return
	}
	clusters, err := s.meta.ListTrustedClusters(true)
	if err != nil {
		return
	}
	seen := make(map[string]struct{})
	for _, rec := range clusters {
		if rec.IsLocal {
			continue
		}
		seen[rec.ID] = struct{}{}
		if !rec.Active || rec.Status == metadata.TrustedClusterStatusRevoked {
			w.setHealthy(rec.ID, false)
			continue
		}
		err := s.healthCheckTrustedCluster(ctx, rec)
		w.setHealthy(rec.ID, err == nil)
	}
	w.dropMissing(seen)
}

func (s *Server) healthCheckTrustedCluster(ctx context.Context, rec metadata.TrustedCluster) error {
	if !rec.Active || rec.Status == metadata.TrustedClusterStatusRevoked {
		return errTrustedClusterRevoked
	}
	endpoint := strings.TrimRight(strings.TrimSpace(rec.Endpoint), "/")
	if endpoint == "" {
		return errors.New("empty peer endpoint")
	}
	if err := urlpolicy.ValidateOutboundURL(endpoint, urlpolicy.DefaultOptions()); err != nil {
		return err
	}
	revoked, _ := s.meta.ListRevokedClusterCertSerials()
	pkiMgr := s.clusterPKI()
	client := &http.Client{Timeout: 10 * time.Second}
	if pkiMgr.HasClientCert(rec.ID) && strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		tlsCfg, err := pkiMgr.ClientTLSConfig(rec.ID, nil, revoked)
		if err == nil {
			client.Transport = &http.Transport{TLSClientConfig: tlsCfg}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return errors.New("peer health check failed")
	}
	return nil
}

func (w *trustedClusterWorker) setHealthy(id string, ok bool) {
	w.mu.Lock()
	w.healthy[id] = ok
	w.mu.Unlock()
}

func (w *trustedClusterWorker) dropMissing(seen map[string]struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id := range w.healthy {
		if _, ok := seen[id]; !ok {
			delete(w.healthy, id)
		}
	}
}

func (w *trustedClusterWorker) isHealthy(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.healthy[id]
}

// HealthCheckTrustedCluster probes a trusted peer (tests and diagnostics).
func (s *Server) HealthCheckTrustedCluster(ctx context.Context, rec metadata.TrustedCluster) error {
	return s.healthCheckTrustedCluster(ctx, rec)
}
