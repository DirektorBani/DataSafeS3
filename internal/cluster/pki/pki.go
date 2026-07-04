package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	caCertFile = "ca.pem"
	caKeyFile  = "ca-key.pem"
	clientDir  = "clients"
)

var (
	ErrInvalidCSR    = errors.New("invalid certificate signing request")
	ErrInvalidCACert = errors.New("invalid CA certificate")
)

// Manager stores cluster PKI material on disk; private keys never go to DB.
type Manager struct {
	dir string
}

func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

func (m *Manager) Dir() string { return m.dir }

func (m *Manager) EnsureCA() error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(m.dir, caCertFile)); err == nil {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"DataSafeS3 Cluster CA"},
			CommonName:   "DataSafeS3 Cluster Root CA",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(m.dir, caCertFile), certPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, caKeyFile), keyPEM, 0o600)
}

func (m *Manager) loadCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(filepath.Join(m.dir, caCertFile))
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(m.dir, caKeyFile))
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("decode ca cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("decode ca key")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func (m *Manager) CACertPEM() ([]byte, error) {
	if err := m.EnsureCA(); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(m.dir, caCertFile))
}

func (m *Manager) CAFingerprint() (string, error) {
	if err := m.EnsureCA(); err != nil {
		return "", err
	}
	certPEM, err := m.CACertPEM()
	if err != nil {
		return "", err
	}
	cert, err := ParseCertificatePEM(certPEM)
	if err != nil {
		return "", err
	}
	return SPKIFingerprint(cert), nil
}

func (m *Manager) GenerateCSR(commonName string) (csrPEM []byte, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	if commonName == "" {
		commonName = "datasafe-cluster-client"
	}
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"DataSafeS3 Cluster"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTmpl, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func (m *Manager) SignCSR(csrPEM []byte, ttl time.Duration) (certPEM []byte, serial string, notBefore, notAfter time.Time, err error) {
	if err := m.EnsureCA(); err != nil {
		return nil, "", time.Time{}, time.Time{}, err
	}
	caCert, caKey, err := m.loadCA()
	if err != nil {
		return nil, "", time.Time{}, time.Time{}, err
	}
	csr, err := ParseCSRPEM(csrPEM)
	if err != nil {
		return nil, "", time.Time{}, time.Time{}, ErrInvalidCSR
	}
	if csr.SignatureAlgorithm != x509.ECDSAWithSHA256 && csr.SignatureAlgorithm != x509.SHA256WithRSA {
		return nil, "", time.Time{}, time.Time{}, ErrInvalidCSR
	}
	serialNum, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, "", time.Time{}, time.Time{}, err
	}
	notBefore = time.Now().UTC().Add(-time.Hour)
	if ttl <= 0 {
		ttl = 90 * 24 * time.Hour
	}
	notAfter = notBefore.Add(ttl)
	tmpl := &x509.Certificate{
		SerialNumber: serialNum,
		Subject:      csr.Subject,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, csr.PublicKey, caKey)
	if err != nil {
		return nil, "", time.Time{}, time.Time{}, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return certPEM, serialNum.Text(16), notBefore, notAfter, nil
}

func (m *Manager) SaveClientCert(clusterID string, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(filepath.Join(m.dir, clientDir), 0o700); err != nil {
		return err
	}
	base := filepath.Join(m.dir, clientDir, sanitizeID(clusterID))
	if err := os.WriteFile(base+".pem", certPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(base+"-key.pem", keyPEM, 0o600)
}

func (m *Manager) VerifyPeerCACert(pemBytes []byte) (fingerprint string, err error) {
	cert, err := ParseCertificatePEM(pemBytes)
	if err != nil {
		return "", ErrInvalidCACert
	}
	if !cert.IsCA {
		return "", ErrInvalidCACert
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return "", ErrInvalidCACert
	}
	return SPKIFingerprint(cert), nil
}

func ParseCertificatePEM(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func ParseCSRPEM(pemBytes []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	return x509.ParseCertificateRequest(block.Bytes)
}

// SPKIFingerprint returns lowercase hex SHA-256 of SubjectPublicKeyInfo (pin peer CA).
func SPKIFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

func sanitizeID(id string) string {
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, id)
	if id == "" {
		return "cluster"
	}
	return id
}

func CertDirFromEnv(dataDir string) string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_CLUSTER_CERT_DIR")); v != "" {
		return v
	}
	if dataDir != "" {
		return filepath.Join(dataDir, "cluster-certs")
	}
	return "./data/cluster-certs"
}
