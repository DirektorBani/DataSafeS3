package metadata

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	TrustedClusterAuthMTLS       = "mtls"
	TrustedClusterStatusHealthy  = "healthy"
	TrustedClusterStatusRenewing = "renewing"
	TrustedClusterStatusRevoked  = "revoked"

	ClusterCertRoleCA     = "ca"
	ClusterCertRoleClient = "client"

	PairingTokenPrefix = "dsjoin_"
	PairingTokenTTL    = 15 * time.Minute
	ClusterCertTTL     = 90 * 24 * time.Hour
)

type TrustedCluster struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Endpoint          string     `json:"endpoint"`
	AuthMode          string     `json:"auth_mode"`
	PeerCAFingerprint string     `json:"peer_ca_fingerprint,omitempty"`
	Status            string     `json:"status"`
	CertExpiresAt     *time.Time `json:"cert_expires_at,omitempty"`
	LastRotatedAt     *time.Time `json:"last_rotated_at,omitempty"`
	NextRotationAt    *time.Time `json:"next_rotation_at,omitempty"`
	SafetyNumber      string     `json:"safety_number,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	Active            bool       `json:"active"`
	IsLocal           bool       `json:"is_local,omitempty"`
}

type ClusterPairingSession struct {
	TokenHash          string     `json:"-"`
	ExpiresAt          time.Time  `json:"expires_at"`
	UsedAt             *time.Time `json:"used_at,omitempty"`
	InitiatorClusterID string     `json:"initiator_cluster_id"`
}

type ClusterCertificate struct {
	Serial    string     `json:"serial"`
	ClusterID string     `json:"cluster_id"`
	Role      string     `json:"role"`
	NotBefore time.Time  `json:"not_before"`
	NotAfter  time.Time  `json:"not_after"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func HashPairingToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func SafetyNumber(localFP, peerFP string) string {
	localFP = strings.ToLower(strings.TrimSpace(localFP))
	peerFP = strings.ToLower(strings.TrimSpace(peerFP))
	if localFP == "" || peerFP == "" {
		return ""
	}
	if localFP > peerFP {
		localFP, peerFP = peerFP, localFP
	}
	sum := sha256.Sum256([]byte(localFP + peerFP))
	return hex.EncodeToString(sum[:4])
}
