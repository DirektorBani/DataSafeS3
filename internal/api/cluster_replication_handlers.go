package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) handleListClusterReplicationRules(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	if _, err := s.meta.GetTrustedCluster(clusterID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	rules, err := s.meta.ListSiteReplicationRulesForTrustedCluster(clusterID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	st, _ := s.meta.SiteReplicationStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"rules":  rules,
		"status": st,
	})
}

func (s *Server) handleCreateClusterReplicationRule(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	rec, err := s.meta.GetTrustedCluster(clusterID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if rec.IsLocal || !rec.Active || rec.Status == metadata.TrustedClusterStatusRevoked {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cluster not eligible for replication"})
		return
	}
	var req struct {
		SourceBucket string `json:"source_bucket"`
		DestBucket   string `json:"dest_bucket"`
		Direction    string `json:"direction"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.SourceBucket = strings.TrimSpace(req.SourceBucket)
	req.DestBucket = strings.TrimSpace(req.DestBucket)
	if req.SourceBucket == "" || req.DestBucket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source_bucket and dest_bucket required"})
		return
	}
	if req.Direction == "" {
		req.Direction = "one-way"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule := metadata.SiteReplicationRule{
		ID:               randomID(),
		TrustedClusterID: clusterID,
		SourceBucket:     req.SourceBucket,
		DestBucket:       req.DestBucket,
		Direction:        req.Direction,
		Enabled:          enabled,
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.meta.PutSiteReplicationRule(rule); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if req.Direction == "bidirectional" {
		s.provisionBidirectionalReplRule(r.Context(), rec, rule)
	}
	s.logActivity(r, "cluster.replication_rule.create", "cluster", clusterID)
	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
}

func (s *Server) handleDeleteClusterReplicationRule(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("id")
	ruleID := r.PathValue("ruleId")
	rules, err := s.meta.ListSiteReplicationRulesForTrustedCluster(clusterID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	found := false
	for _, rule := range rules {
		if rule.ID == ruleID {
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err := s.meta.DeleteSiteReplicationRule(ruleID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": ruleID})
}

func (s *Server) provisionBidirectionalReplRule(ctx context.Context, peer metadata.TrustedCluster, localRule metadata.SiteReplicationRule) {
	if !s.clusterPKI().HasClientCert(peer.ID) {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"source_bucket": localRule.DestBucket,
		"dest_bucket":   localRule.SourceBucket,
		"direction":     "one-way",
	})
	url := strings.TrimRight(peer.Endpoint, "/") + "/api/v1/clusters/" + s.localClusterID() + "/replication-rules"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.clusterPeerHTTPClient(ctx, peer.ID, peer.Endpoint).Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
}

func (s *Server) clusterPeerHTTPClient(ctx context.Context, clusterID, endpoint string) *http.Client {
	_ = ctx
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	client := &http.Client{Timeout: 30 * time.Second}
	pkiMgr := s.clusterPKI()
	if pkiMgr.HasClientCert(clusterID) && strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		revoked, _ := s.meta.ListRevokedClusterCertSerials()
		if tlsCfg, err := pkiMgr.ClientTLSConfig(clusterID, nil, revoked); err == nil {
			client.Transport = &http.Transport{TLSClientConfig: tlsCfg}
		}
	}
	return client
}

func (s *Server) handleClusterRevokeNotify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != s.localClusterID() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var body struct {
		RevokedClusterID string `json:"revoked_cluster_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.RevokedClusterID = strings.TrimSpace(body.RevokedClusterID)
	if body.RevokedClusterID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "revoked_cluster_id required"})
		return
	}
	peerRec, err := s.meta.GetTrustedCluster(body.RevokedClusterID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown peer"})
		return
	}
	now := time.Now().UTC()
	certs, _ := s.meta.ListClusterCertificates(body.RevokedClusterID)
	for _, c := range certs {
		if c.RevokedAt == nil {
			_ = s.meta.RevokeClusterCertificate(c.Serial, now)
		}
	}
	_ = s.meta.DeactivateTrustedCluster(body.RevokedClusterID)
	s.logActivityAs("system", clientIP(r), "cluster.revoke_notify", "cluster", peerRec.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleClusterRotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != s.localClusterID() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		CSRPEM string `json:"csr_pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.CSRPEM) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "csr_pem required"})
		return
	}
	pkiMgr := s.clusterPKI()
	if err := pkiMgr.EnsureCA(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	certPEM, serial, nb, na, err := pkiMgr.SignCSR([]byte(req.CSRPEM), metadata.ClusterCertTTL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to sign CSR"})
		return
	}
	_ = s.meta.PutClusterCertificate(metadata.ClusterCertificate{
		Serial: serial, ClusterID: id, Role: metadata.ClusterCertRoleClient,
		NotBefore: nb, NotAfter: na,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"signed_client_cert": string(certPEM),
		"serial":             serial,
		"cert_expires_at":    na,
	})
}

func (s *Server) notifyPeerClusterRevoke(ctx context.Context, rec metadata.TrustedCluster) {
	if !s.clusterPKI().HasClientCert(rec.ID) {
		return
	}
	payload, _ := json.Marshal(map[string]string{"revoked_cluster_id": s.localClusterID()})
	url := strings.TrimRight(rec.Endpoint, "/") + "/api/v1/clusters/" + rec.ID + "/revoke-notify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.clusterPeerHTTPClient(ctx, rec.ID, rec.Endpoint).Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
}

func (s *Server) requestPeerClusterCertRotation(ctx context.Context, rec metadata.TrustedCluster, csrPEM []byte) ([]byte, string, time.Time, error) {
	if !s.clusterPKI().HasClientCert(rec.ID) {
		return nil, "", time.Time{}, errors.New("no client cert for peer")
	}
	payload, _ := json.Marshal(map[string]string{"csr_pem": string(csrPEM)})
	url := strings.TrimRight(rec.Endpoint, "/") + "/api/v1/clusters/" + s.localClusterID() + "/rotate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.clusterPeerHTTPClient(ctx, rec.ID, rec.Endpoint).Do(req)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, "", time.Time{}, errors.New(string(body))
	}
	var out struct {
		SignedClientCert string    `json:"signed_client_cert"`
		Serial           string    `json:"serial"`
		CertExpiresAt    time.Time `json:"cert_expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", time.Time{}, err
	}
	if out.SignedClientCert == "" {
		return nil, "", time.Time{}, errors.New("empty signed cert from peer")
	}
	return []byte(out.SignedClientCert), out.Serial, out.CertExpiresAt, nil
}

// ProcessTrustedClusterPeersOnce runs one health-check cycle (integration tests).
func (s *Server) ProcessTrustedClusterPeersOnce(ctx context.Context) {
	if s.trustedClusterW == nil {
		s.trustedClusterW = &trustedClusterWorker{healthy: make(map[string]bool)}
	}
	s.processTrustedClusterPeersOnce(ctx, s.trustedClusterW)
}
