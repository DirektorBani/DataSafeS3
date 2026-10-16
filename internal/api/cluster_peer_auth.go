package api

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type clusterPeerCtxKey struct{}

const (
	headerClusterCert = "X-DataSafe-Cluster-Cert"
	headerClusterSig  = "X-DataSafe-Cluster-Sig"
)

// requireClusterPeer rejects rotate and revoke-notify unless the caller proves
// possession of a client certificate this cluster issued to a trusted peer.
// The listener is plain HTTP, so the proof is a request signature (or a TLS
// client certificate when the connection actually has one).
func (s *Server) requireClusterPeer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		peerID, ok := s.authenticateClusterPeer(r, body)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		ctx := context.WithValue(r.Context(), clusterPeerCtxKey{}, peerID)
		next(w, r.WithContext(ctx))
	}
}

func clusterPeerID(r *http.Request) string {
	id, _ := r.Context().Value(clusterPeerCtxKey{}).(string)
	return id
}

func (s *Server) authenticateClusterPeer(r *http.Request, body []byte) (string, bool) {
	if r.TLS != nil {
		for _, cert := range r.TLS.PeerCertificates {
			if id, ok := s.trustedIssuedCert(cert); ok {
				return id, true
			}
		}
	}
	certPEM, err := base64.StdEncoding.DecodeString(strings.TrimSpace(r.Header.Get(headerClusterCert)))
	if err != nil || len(certPEM) == 0 {
		return "", false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(r.Header.Get(headerClusterSig)))
	if err != nil || len(sig) == 0 {
		return "", false
	}
	cert, err := pki.ParseCertificatePEM(certPEM)
	if err != nil {
		return "", false
	}
	if err := pki.VerifyCanonical(cert, pki.CanonicalClusterRequest(r.Method, r.URL.Path, body), sig); err != nil {
		return "", false
	}
	return s.trustedIssuedCert(cert)
}

func (s *Server) trustedIssuedCert(cert *x509.Certificate) (string, bool) {
	if cert == nil {
		return "", false
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return "", false
	}
	if err := s.clusterPKI().SignedByCA(cert); err != nil {
		return "", false
	}
	serial := strings.ToLower(cert.SerialNumber.Text(16))
	revoked, _ := s.meta.ListRevokedClusterCertSerials()
	for _, srl := range revoked {
		if strings.EqualFold(srl, serial) {
			return "", false
		}
	}
	certs, err := s.meta.ListClusterCertificates("")
	if err != nil {
		return "", false
	}
	var clusterID string
	found := false
	for _, rec := range certs {
		if strings.EqualFold(rec.Serial, serial) && rec.RevokedAt == nil {
			clusterID = rec.ClusterID
			found = true
			break
		}
	}
	if !found || clusterID == "" || clusterID == s.localClusterID() {
		return "", false
	}
	peer, err := s.meta.GetTrustedCluster(clusterID)
	if err != nil || !peer.Active || peer.Status == metadata.TrustedClusterStatusRevoked {
		return "", false
	}
	return clusterID, true
}

func (s *Server) signClusterPeerRequest(req *http.Request, peerID string, body []byte) {
	certPEM, keyPEM, err := s.clusterPKI().ClientMaterial(peerID)
	if err != nil {
		return
	}
	sig, err := pki.SignCanonical(keyPEM, pki.CanonicalClusterRequest(req.Method, req.URL.Path, body))
	if err != nil {
		return
	}
	req.Header.Set(headerClusterCert, base64.StdEncoding.EncodeToString(certPEM))
	req.Header.Set(headerClusterSig, base64.StdEncoding.EncodeToString(sig))
}
