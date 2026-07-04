package api

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

const clusterCertDualTrustWindow = 24 * time.Hour

func clusterCertRenewBeforeDays() time.Duration {
	days := 75
	if v := strings.TrimSpace(os.Getenv("STORAGE_CLUSTER_CERT_RENEW_BEFORE_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	return time.Duration(days) * 24 * time.Hour
}

func clusterCertRotatorInterval() time.Duration {
	if v := strings.TrimSpace(os.Getenv("STORAGE_CLUSTER_CERT_ROTATOR_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return time.Hour
}

func (s *Server) runClusterCertRotator(ctx context.Context) {
	ticker := time.NewTicker(clusterCertRotatorInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processClusterCertRotationOnce(ctx)
		}
	}
}

func (s *Server) processClusterCertRotationOnce(ctx context.Context) {
	if s.haLeader != nil && s.haLeader.Enabled() && !s.haLeader.IsLeader(ctx) {
		return
	}
	pkiMgr := s.clusterPKI()
	if err := pkiMgr.EnsureCA(); err != nil {
		return
	}
	clusters, err := s.meta.ListTrustedClusters(true)
	if err != nil {
		return
	}
	renewBefore := clusterCertRenewBeforeDays()
	now := time.Now().UTC()
	for _, rec := range clusters {
		if rec.IsLocal || !rec.Active || rec.AuthMode != metadata.TrustedClusterAuthMTLS {
			continue
		}
		if !pkiMgr.HasClientCert(rec.ID) {
			continue
		}
		due := rec.NextRotationAt != nil && !now.Before(*rec.NextRotationAt)
		if !due && rec.CertExpiresAt != nil {
			due = now.After(rec.CertExpiresAt.Add(-renewBefore))
		}
		if !due {
			s.revokeExpiredDualTrustCerts(rec.ID, now)
			continue
		}
		_ = s.rotateTrustedClusterCert(rec, now, renewBefore)
		s.revokeExpiredDualTrustCerts(rec.ID, now)
	}
}

func (s *Server) rotateTrustedClusterCert(rec metadata.TrustedCluster, now time.Time, renewBefore time.Duration) error {
	pkiMgr := s.clusterPKI()
	rec.Status = metadata.TrustedClusterStatusRenewing
	_ = s.meta.PutTrustedCluster(rec)

	csrPEM, keyPEM, err := pkiMgr.GenerateCSR(rec.ID)
	if err != nil {
		rec.Status = metadata.TrustedClusterStatusHealthy
		_ = s.meta.PutTrustedCluster(rec)
		return err
	}
	var certPEM []byte
	var serial string
	var nb, na time.Time
	if pkiMgr.HasClientCert(rec.ID) {
		if peerCert, peerSerial, peerExp, peerErr := s.requestPeerClusterCertRotation(context.Background(), rec, csrPEM); peerErr == nil {
			certPEM = peerCert
			serial = peerSerial
			na = peerExp
			nb = now
		}
	}
	if len(certPEM) == 0 {
		certPEM, serial, nb, na, err = pkiMgr.SignCSR(csrPEM, metadata.ClusterCertTTL)
		if err != nil {
			rec.Status = metadata.TrustedClusterStatusHealthy
			_ = s.meta.PutTrustedCluster(rec)
			return err
		}
	}
	if err := pkiMgr.SaveClientCert(rec.ID, certPEM, keyPEM); err != nil {
		rec.Status = metadata.TrustedClusterStatusHealthy
		_ = s.meta.PutTrustedCluster(rec)
		return err
	}
	certRec := metadata.ClusterCertificate{
		Serial:    serial,
		ClusterID: rec.ID,
		Role:      metadata.ClusterCertRoleClient,
		NotBefore: nb,
		NotAfter:  na,
	}
	if err := s.meta.PutClusterCertificate(certRec); err != nil {
		return err
	}
	rec.CertExpiresAt = &na
	rec.LastRotatedAt = &now
	next := nb.Add(renewBefore)
	rec.NextRotationAt = &next
	rec.Status = metadata.TrustedClusterStatusHealthy
	return s.meta.PutTrustedCluster(rec)
}

func (s *Server) revokeExpiredDualTrustCerts(clusterID string, now time.Time) {
	certs, err := s.meta.ListClusterCertificates(clusterID)
	if err != nil {
		return
	}
	var active []metadata.ClusterCertificate
	for _, c := range certs {
		if c.RevokedAt == nil && c.Role == metadata.ClusterCertRoleClient {
			active = append(active, c)
		}
	}
	if len(active) <= 1 {
		return
	}
	newest := active[0]
	for _, c := range active[1:] {
		if c.NotBefore.After(newest.NotBefore) {
			newest = c
		}
	}
	for _, c := range active {
		if c.Serial == newest.Serial {
			continue
		}
		retireAt := c.NotBefore.Add(clusterCertDualTrustWindow)
		if now.Before(retireAt) {
			continue
		}
		_ = s.meta.RevokeClusterCertificate(c.Serial, now)
	}
}

// ProcessClusterCertRotationOnce runs one cert-rotation cycle (integration tests).
func (s *Server) ProcessClusterCertRotationOnce(ctx context.Context) {
	s.processClusterCertRotationOnce(ctx)
}
