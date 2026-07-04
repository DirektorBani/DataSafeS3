package pki

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrRevokedCertSerial = errors.New("certificate serial revoked")
	ErrNoClientCert      = errors.New("cluster client certificate not found")
)

const peerCADir = "peers"

// HasClientCert reports whether a client cert/key pair exists for clusterID.
func (m *Manager) HasClientCert(clusterID string) bool {
	base := filepath.Join(m.dir, clientDir, sanitizeID(clusterID))
	if _, err := os.Stat(base + ".pem"); err != nil {
		return false
	}
	_, err := os.Stat(base + "-key.pem")
	return err == nil
}

func (m *Manager) clientCertPaths(clusterID string) (certPath, keyPath string) {
	base := filepath.Join(m.dir, clientDir, sanitizeID(clusterID))
	return base + ".pem", base + "-key.pem"
}

// LoadClientCertificate loads the stored client cert/key for clusterID.
func (m *Manager) LoadClientCertificate(clusterID string) (tls.Certificate, error) {
	certPath, keyPath := m.clientCertPaths(clusterID)
	return tls.LoadX509KeyPair(certPath, keyPath)
}

// SavePeerCA stores the peer cluster CA certificate (public material).
func (m *Manager) SavePeerCA(clusterID string, caPEM []byte) error {
	if err := os.MkdirAll(filepath.Join(m.dir, peerCADir), 0o700); err != nil {
		return err
	}
	path := filepath.Join(m.dir, peerCADir, sanitizeID(clusterID)+"-ca.pem")
	return os.WriteFile(path, caPEM, 0o644)
}

// PeerCAPEM returns stored peer CA certificate PEM.
func (m *Manager) PeerCAPEM(clusterID string) ([]byte, error) {
	path := filepath.Join(m.dir, peerCADir, sanitizeID(clusterID)+"-ca.pem")
	return os.ReadFile(path)
}

func revokedSerialSet(revoked []string) map[string]struct{} {
	out := make(map[string]struct{}, len(revoked))
	for _, s := range revoked {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out[s] = struct{}{}
		}
	}
	return out
}

// CheckSerialRevoked returns ErrRevokedCertSerial when serial is in revoked list.
func CheckSerialRevoked(serial string, revoked map[string]struct{}) error {
	serial = strings.ToLower(strings.TrimSpace(serial))
	if serial == "" {
		return nil
	}
	if _, ok := revoked[serial]; ok {
		return ErrRevokedCertSerial
	}
	return nil
}

// ClientTLSConfig builds mTLS config for outbound connections to a trusted cluster peer.
func (m *Manager) ClientTLSConfig(clusterID string, peerCAPEM []byte, revokedSerials []string) (*tls.Config, error) {
	if len(peerCAPEM) == 0 {
		var err error
		peerCAPEM, err = m.PeerCAPEM(clusterID)
		if err != nil {
			return nil, err
		}
	}
	cert, err := m.LoadClientCertificate(clusterID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoClientCert, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(peerCAPEM) {
		return nil, fmt.Errorf("invalid peer CA PEM")
	}
	revoked := revokedSerialSet(revokedSerials)
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		VerifyConnection: func(cs tls.ConnectionState) error {
			for _, c := range cs.PeerCertificates {
				if err := CheckSerialRevoked(strings.ToLower(c.SerialNumber.Text(16)), revoked); err != nil {
					return err
				}
			}
			return nil
		},
	}, nil
}

// VerifyClientSerial rejects TLS client certs whose serial appears in revoked list.
func VerifyClientSerial(rawCerts [][]byte, verifiedChains [][]*x509.Certificate, revokedSerials []string) error {
	revoked := revokedSerialSet(revokedSerials)
	for _, chain := range verifiedChains {
		for _, cert := range chain {
			if err := CheckSerialRevoked(strings.ToLower(cert.SerialNumber.Text(16)), revoked); err != nil {
				return err
			}
		}
	}
	for _, raw := range rawCerts {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return err
		}
		if err := CheckSerialRevoked(strings.ToLower(cert.SerialNumber.Text(16)), revoked); err != nil {
			return err
		}
	}
	return nil
}

// ParseCAPEMFromFile reads a PEM CA certificate from disk.
func ParseCAPEMFromFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", path)
	}
	return data, nil
}
