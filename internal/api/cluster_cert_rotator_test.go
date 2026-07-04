package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestClusterCert_autoRotate_beforeExpiry(t *testing.T) {
	t.Setenv("STORAGE_CLUSTER_CERT_RENEW_BEFORE_DAYS", "1")
	s := testServer(t)
	pkiMgr := pki.NewManager(t.TempDir())
	clusterID := "remote-test"
	csrPEM, keyPEM, err := pkiMgr.GenerateCSR(clusterID)
	if err != nil {
		t.Fatal(err)
	}
	shortTTL := 2 * time.Hour
	certPEM, serial, nb, na, err := pkiMgr.SignCSR(csrPEM, shortTTL)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkiMgr.SaveClientCert(clusterID, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rec := metadata.TrustedCluster{
		ID:             clusterID,
		Name:           "Remote",
		Endpoint:       "http://127.0.0.1:9000",
		AuthMode:       metadata.TrustedClusterAuthMTLS,
		Status:         metadata.TrustedClusterStatusHealthy,
		CertExpiresAt:  &na,
		LastRotatedAt:  &nb,
		NextRotationAt: clusterPtrTime(now.Add(-time.Minute)),
		CreatedAt:      now,
		Active:         true,
	}
	if err := s.Meta().PutTrustedCluster(rec); err != nil {
		t.Fatal(err)
	}
	_ = s.Meta().PutClusterCertificate(metadata.ClusterCertificate{
		Serial: serial, ClusterID: clusterID, Role: metadata.ClusterCertRoleClient,
		NotBefore: nb, NotAfter: na,
	})

	// Patch server PKI dir via env used by clusterPKI
	t.Setenv("STORAGE_CLUSTER_CERT_DIR", pkiMgr.Dir())
	s.ProcessClusterCertRotationOnce(context.Background())

	updated, err := s.Meta().GetTrustedCluster(clusterID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastRotatedAt == nil || !updated.LastRotatedAt.After(nb) {
		t.Fatalf("expected rotation to update last_rotated_at")
	}
	if updated.CertExpiresAt == nil || !updated.CertExpiresAt.After(na) {
		t.Fatalf("expected new cert expiry after rotation")
	}
	certs, _ := s.Meta().ListClusterCertificates(clusterID)
	if len(certs) < 2 {
		t.Fatalf("expected dual-trust cert records, got %d", len(certs))
	}
}

func clusterPtrTime(t time.Time) *time.Time { return &t }
