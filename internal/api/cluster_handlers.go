package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

func (s *Server) clusterPKI() *pki.Manager {
	if s.clusterPKI_ == nil {
		s.clusterPKI_ = pki.NewManager(pki.CertDirFromEnv(s.cfg.DataDir))
	}
	return s.clusterPKI_
}

func (s *Server) localClusterID() string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_CLUSTER_ID")); v != "" {
		return v
	}
	return "local"
}

func (s *Server) localClusterEndpoint(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_CLUSTER_ENDPOINT")); v != "" {
		return strings.TrimRight(v, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:9000"
	}
	return scheme + "://" + host
}

func (s *Server) ensureLocalTrustedCluster(r *http.Request) (metadata.TrustedCluster, error) {
	pkiMgr := s.clusterPKI()
	if err := pkiMgr.EnsureCA(); err != nil {
		return metadata.TrustedCluster{}, err
	}
	id := s.localClusterID()
	rec, err := s.meta.GetTrustedCluster(id)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, metadata.ErrNotFound) {
		return rec, err
	}
	now := time.Now().UTC()
	rec = metadata.TrustedCluster{
		ID:        id,
		Name:      "Local cluster",
		Endpoint:  s.localClusterEndpoint(r),
		AuthMode:  metadata.TrustedClusterAuthMTLS,
		Status:    metadata.TrustedClusterStatusHealthy,
		CreatedAt: now,
		Active:    true,
		IsLocal:   true,
	}
	return rec, s.meta.PutTrustedCluster(rec)
}

func (s *Server) handleCreateClusterPairingCode(w http.ResponseWriter, r *http.Request) {
	if _, err := s.ensureLocalTrustedCluster(r); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	raw, err := randomPairingToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token generation failed"})
		return
	}
	hash := metadata.HashPairingToken(raw)
	expires := time.Now().UTC().Add(metadata.PairingTokenTTL)
	session := metadata.ClusterPairingSession{
		TokenHash:          hash,
		ExpiresAt:          expires,
		InitiatorClusterID: s.localClusterID(),
	}
	if err := s.meta.PutClusterPairingSession(session); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, "cluster.pairing_code.create", "cluster", s.localClusterID())
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      raw,
		"expires_at": expires,
		"qr_payload": raw,
	})
}

func (s *Server) handleClusterPairComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token          string `json:"token"`
		JoinerName     string `json:"joiner_name"`
		JoinerEndpoint string `json:"joiner_endpoint"`
		JoinerCAPEM    string `json:"joiner_ca_pem"`
		JoinerCSRPEM   string `json:"joiner_csr_pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || req.JoinerCSRPEM == "" || req.JoinerCAPEM == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token, joiner_ca_pem, joiner_csr_pem required"})
		return
	}
	if req.JoinerEndpoint != "" {
		if err := urlpolicy.ValidateOutboundURL(req.JoinerEndpoint, clusterPeerURLPolicy()); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
	}
	hash := metadata.HashPairingToken(req.Token)
	session, err := s.meta.GetClusterPairingSession(hash)
	if err != nil {
		s.logActivityAs("system", clientIP(r), "cluster.pair.failed", "cluster", "invalid_token")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired pairing token"})
		return
	}
	now := time.Now().UTC()
	if session.UsedAt != nil {
		s.logActivityAs("system", clientIP(r), "cluster.pair.failed", "cluster", "replay")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "pairing token already used"})
		return
	}
	if now.After(session.ExpiresAt) {
		s.logActivityAs("system", clientIP(r), "cluster.pair.failed", "cluster", "expired")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "pairing token expired"})
		return
	}
	pkiMgr := s.clusterPKI()
	peerFP, err := pkiMgr.VerifyPeerCACert([]byte(req.JoinerCAPEM))
	if err != nil {
		s.logActivityAs("system", clientIP(r), "cluster.pair.failed", "cluster", "bad_ca")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid joiner CA certificate"})
		return
	}
	certPEM, serial, nb, na, err := pkiMgr.SignCSR([]byte(req.JoinerCSRPEM), metadata.ClusterCertTTL)
	if err != nil {
		s.logActivityAs("system", clientIP(r), "cluster.pair.failed", "cluster", "csr_sign")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to sign joiner CSR"})
		return
	}
	localFP, err := pkiMgr.CAFingerprint()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	clusterID := randomID()
	name := strings.TrimSpace(req.JoinerName)
	if name == "" {
		name = "Remote cluster " + clusterID
	}
	endpoint := strings.TrimRight(strings.TrimSpace(req.JoinerEndpoint), "/")
	safety := metadata.SafetyNumber(localFP, peerFP)
	rec := metadata.TrustedCluster{
		ID:                clusterID,
		Name:              name,
		Endpoint:          endpoint,
		AuthMode:          metadata.TrustedClusterAuthMTLS,
		PeerCAFingerprint: peerFP,
		Status:            metadata.TrustedClusterStatusHealthy,
		CertExpiresAt:     &na,
		LastRotatedAt:     &nb,
		NextRotationAt:    clusterPtrTime(nb.Add(75 * 24 * time.Hour)),
		SafetyNumber:      safety,
		CreatedAt:         now,
		Active:            true,
	}
	if err := s.meta.PutTrustedCluster(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	certRec := metadata.ClusterCertificate{
		Serial:    serial,
		ClusterID: clusterID,
		Role:      metadata.ClusterCertRoleClient,
		NotBefore: nb,
		NotAfter:  na,
	}
	_ = s.meta.PutClusterCertificate(certRec)
	_ = pkiMgr.SavePeerCA(clusterID, []byte(req.JoinerCAPEM))
	if err := s.meta.MarkClusterPairingSessionUsed(hash, now); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	caPEM, _ := pkiMgr.CACertPEM()
	s.logActivityAs("system", clientIP(r), "cluster.pair.success", "cluster", clusterID)
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster_id":          clusterID,
		"initiator_ca_pem":    string(caPEM),
		"signed_client_cert":  string(certPEM),
		"safety_number":       safety,
		"peer_ca_fingerprint": peerFP,
		"cert_expires_at":     na,
	})
}

func (s *Server) handleClusterPairJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InitiatorURL string `json:"initiator_url"`
		Token        string `json:"token"`
		Name         string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.InitiatorURL = strings.TrimRight(strings.TrimSpace(req.InitiatorURL), "/")
	req.Token = strings.TrimSpace(req.Token)
	if req.InitiatorURL == "" || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "initiator_url and token required"})
		return
	}
	if err := urlpolicy.ValidateOutboundURL(req.InitiatorURL, clusterPeerURLPolicy()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	pkiMgr := s.clusterPKI()
	if err := pkiMgr.EnsureCA(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	csrPEM, keyPEM, err := pkiMgr.GenerateCSR(s.localClusterID())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	caPEM, err := pkiMgr.CACertPEM()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"token":           req.Token,
		"joiner_name":     strings.TrimSpace(req.Name),
		"joiner_endpoint": s.localClusterEndpoint(r),
		"joiner_ca_pem":   string(caPEM),
		"joiner_csr_pem":  string(csrPEM),
	})
	completeURL := req.InitiatorURL + "/api/v1/clusters/pair/complete"
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, completeURL, bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		s.logActivity(r, "cluster.pair.failed", "cluster", req.InitiatorURL)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		s.logActivity(r, "cluster.pair.failed", "cluster", req.InitiatorURL)
		var errBody map[string]any
		_ = json.Unmarshal(body, &errBody)
		if errBody == nil {
			errBody = map[string]any{"error": string(body)}
		}
		writeJSON(w, resp.StatusCode, errBody)
		return
	}
	var completeResp struct {
		ClusterID         string    `json:"cluster_id"`
		InitiatorCAPEM    string    `json:"initiator_ca_pem"`
		SignedClientCert  string    `json:"signed_client_cert"`
		SafetyNumber      string    `json:"safety_number"`
		PeerCAFingerprint string    `json:"peer_ca_fingerprint"`
		CertExpiresAt     time.Time `json:"cert_expires_at"`
	}
	if err := json.Unmarshal(body, &completeResp); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "invalid initiator response"})
		return
	}
	peerFP, err := pkiMgr.VerifyPeerCACert([]byte(completeResp.InitiatorCAPEM))
	if err != nil {
		s.logActivity(r, "cluster.pair.failed", "cluster", "initiator_ca")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "initiator CA verification failed"})
		return
	}
	localFP, _ := pkiMgr.CAFingerprint()
	clusterID := completeResp.ClusterID
	if clusterID == "" {
		clusterID = randomID()
	}
	if err := pkiMgr.SaveClientCert(clusterID, []byte(completeResp.SignedClientCert), keyPEM); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = pkiMgr.SavePeerCA(clusterID, []byte(completeResp.InitiatorCAPEM))
	now := time.Now().UTC()
	exp := completeResp.CertExpiresAt
	if exp.IsZero() {
		exp = now.Add(metadata.ClusterCertTTL)
	}
	safety := completeResp.SafetyNumber
	if safety == "" {
		safety = metadata.SafetyNumber(localFP, peerFP)
	}
	rec := metadata.TrustedCluster{
		ID:                clusterID,
		Name:              strings.TrimSpace(req.Name),
		Endpoint:          req.InitiatorURL,
		AuthMode:          metadata.TrustedClusterAuthMTLS,
		PeerCAFingerprint: peerFP,
		Status:            metadata.TrustedClusterStatusHealthy,
		CertExpiresAt:     &exp,
		LastRotatedAt:     &now,
		NextRotationAt:    clusterPtrTime(now.Add(75 * 24 * time.Hour)),
		SafetyNumber:      safety,
		CreatedAt:         now,
		Active:            true,
	}
	if rec.Name == "" {
		rec.Name = "Remote cluster " + clusterID
	}
	if err := s.meta.PutTrustedCluster(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, "cluster.pair.success", "cluster", clusterID)
	writeJSON(w, http.StatusOK, map[string]any{"cluster": rec})
}

func (s *Server) handleListTrustedClusters(w http.ResponseWriter, r *http.Request) {
	local, _ := s.ensureLocalTrustedCluster(r)
	remote, err := s.meta.ListTrustedClusters(true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var clusters []metadata.TrustedCluster
	clusters = append(clusters, local)
	for _, c := range remote {
		if c.IsLocal || c.ID == local.ID {
			continue
		}
		clusters = append(clusters, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters})
}

func (s *Server) handleGetTrustedCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetTrustedCluster(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	certs, _ := s.meta.ListClusterCertificates(id)
	writeJSON(w, http.StatusOK, map[string]any{"cluster": rec, "certificates": certs})
}

func (s *Server) handleRevokeTrustedCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == s.localClusterID() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot revoke local cluster"})
		return
	}
	rec, err := s.meta.GetTrustedCluster(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	now := time.Now().UTC()
	certs, _ := s.meta.ListClusterCertificates(id)
	for _, c := range certs {
		if c.RevokedAt == nil {
			_ = s.meta.RevokeClusterCertificate(c.Serial, now)
		}
	}
	if err := s.meta.DeactivateTrustedCluster(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	go s.notifyPeerClusterRevoke(r.Context(), rec)
	s.logActivity(r, "cluster.revoke", "cluster", rec.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": metadata.TrustedClusterStatusRevoked})
}

func randomPairingToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return metadata.PairingTokenPrefix + hex.EncodeToString(b), nil
}

func clusterPeerURLPolicy() urlpolicy.Options {
	return urlpolicy.DefaultOptions()
}

func clusterPtrTime(t time.Time) *time.Time { return &t }

func federationDeprecatedHeader(w http.ResponseWriter) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `</api/v1/clusters>; rel="successor-version"`)
}
