package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestClusterRevoke_stopsWorkerHealth(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	s := testServer(t)
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer peerSrv.Close()

	clusterID := "revoke-peer"
	pkiMgr := pki.NewManager(t.TempDir())
	csrPEM, keyPEM, err := pkiMgr.GenerateCSR(clusterID)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, serial, nb, na, err := pkiMgr.SignCSR(csrPEM, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkiMgr.SaveClientCert(clusterID, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	if err := pkiMgr.SavePeerCA(clusterID, mustCAPEM(t, pkiMgr)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORAGE_CLUSTER_CERT_DIR", pkiMgr.Dir())

	now := time.Now().UTC()
	rec := metadata.TrustedCluster{
		ID: clusterID, Name: "Peer", Endpoint: peerSrv.URL,
		AuthMode: metadata.TrustedClusterAuthMTLS, Status: metadata.TrustedClusterStatusHealthy,
		CertExpiresAt: &na, LastRotatedAt: &nb, CreatedAt: now, Active: true,
	}
	if err := s.Meta().PutTrustedCluster(rec); err != nil {
		t.Fatal(err)
	}
	_ = s.Meta().PutClusterCertificate(metadata.ClusterCertificate{
		Serial: serial, ClusterID: clusterID, Role: metadata.ClusterCertRoleClient,
		NotBefore: nb, NotAfter: na,
	})

	if err := s.HealthCheckTrustedCluster(context.Background(), rec); err != nil {
		t.Fatalf("pre-revoke health check: %v", err)
	}

	tok := loginToken(t, s, "admin", "admin")
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/"+clusterID+"/revoke", tok, nil)
	recResp := httptest.NewRecorder()
	s.Handler().ServeHTTP(recResp, req)
	if recResp.Code != http.StatusOK {
		t.Fatalf("revoke %d %s", recResp.Code, recResp.Body.String())
	}

	updated, err := s.Meta().GetTrustedCluster(clusterID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.HealthCheckTrustedCluster(context.Background(), updated); err == nil {
		t.Fatal("expected health check failure after revoke")
	}
}

func mustCAPEM(t *testing.T, m *pki.Manager) []byte {
	t.Helper()
	pemBytes, err := m.CACertPEM()
	if err != nil {
		t.Fatal(err)
	}
	return pemBytes
}
