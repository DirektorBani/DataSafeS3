package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func trustedClusterReplEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_TRUSTED_CLUSTER_REPL_ENABLED")))
	if v == "false" || v == "0" {
		return false
	}
	return true
}

func siteReplWorkerEnabled() bool {
	return siteReplEnabled() || trustedClusterReplEnabled()
}

func (s *Server) trustedClusterHealthy(clusterID string) bool {
	if s.trustedClusterW == nil {
		return true
	}
	return s.trustedClusterW.isHealthy(clusterID)
}

func (s *Server) trustedClusterS3Client(ctx context.Context, clusterID, endpoint string) (*s3.Client, error) {
	endpoint = strings.TrimSuffix(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("empty trusted cluster endpoint")
	}
	if err := urlpolicy.ValidateOutboundURL(endpoint, urlpolicy.DefaultOptions()); err != nil {
		return nil, err
	}
	revoked, _ := s.meta.ListRevokedClusterCertSerials()
	pkiMgr := s.clusterPKI()
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	if pkiMgr.HasClientCert(clusterID) && strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		tlsCfg, err := pkiMgr.ClientTLSConfig(clusterID, nil, revoked)
		if err != nil {
			return nil, err
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	} else if strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return nil, errors.New("trusted cluster client certificate required for https replication")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s.cfg.AccessKey, s.cfg.SecretKey, "")),
		awsconfig.WithRegion(s.cfg.Region),
		awsconfig.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}

func (s *Server) siteReplS3ClientForRule(ctx context.Context, rule metadata.SiteReplicationRule) (*s3.Client, string, error) {
	if rule.TrustedClusterID != "" {
		rec, err := s.meta.GetTrustedCluster(rule.TrustedClusterID)
		if err != nil {
			return nil, "", err
		}
		if !rec.Active || rec.Status == metadata.TrustedClusterStatusRevoked {
			return nil, "", errTrustedClusterRevoked
		}
		if !s.trustedClusterHealthy(rule.TrustedClusterID) {
			return nil, "", errors.New("trusted cluster peer unhealthy")
		}
		client, err := s.trustedClusterS3Client(ctx, rule.TrustedClusterID, rec.Endpoint)
		return client, rec.Endpoint, err
	}
	peer, err := s.meta.GetSiteReplicationPeer(rule.PeerID)
	if err != nil {
		return nil, "", err
	}
	if err := urlpolicy.ValidateOutboundURL(peer.Endpoint, urlpolicy.DefaultOptions()); err != nil {
		return nil, "", err
	}
	client, err := siteReplS3Client(peer)
	return client, peer.Endpoint, err
}

// ProcessSiteReplicationOnce runs one replication cycle (integration tests).
func (s *Server) ProcessSiteReplicationOnce(ctx context.Context) {
	s.processSiteReplicationOnce(ctx)
}

// EnqueueSiteReplicationForTest enqueues replication tasks (integration tests).
func (s *Server) EnqueueSiteReplicationForTest(event, bucket, key string) {
	s.enqueueSiteReplication(event, bucket, key)
}

// TrustedClusterHealthyForTest reports peer health (integration tests).
func (s *Server) TrustedClusterHealthyForTest(clusterID string) bool {
	return s.trustedClusterHealthy(clusterID)
}
