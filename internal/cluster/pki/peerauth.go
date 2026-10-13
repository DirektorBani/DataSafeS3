package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// ClientMaterial returns the stored client certificate and private key PEM.
func (m *Manager) ClientMaterial(clusterID string) (certPEM, keyPEM []byte, err error) {
	certPath, keyPath := m.clientCertPaths(clusterID)
	certPEM, err = os.ReadFile(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNoClientCert, err)
	}
	keyPEM, err = os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNoClientCert, err)
	}
	return certPEM, keyPEM, nil
}

// SignedByCA reports whether cert was issued by this node's cluster CA.
func (m *Manager) SignedByCA(cert *x509.Certificate) error {
	if err := m.EnsureCA(); err != nil {
		return err
	}
	ca, _, err := m.loadCA()
	if err != nil {
		return err
	}
	return cert.CheckSignatureFrom(ca)
}

// SignCanonical signs method, path and body with the cluster client key.
func SignCanonical(keyPEM []byte, canonical string) ([]byte, error) {
	key, err := parseECPrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(canonical))
	return ecdsa.SignASN1(rand.Reader, key, sum[:])
}

// VerifyCanonical checks an ASN.1 ECDSA signature over canonical.
func VerifyCanonical(cert *x509.Certificate, canonical string, sig []byte) error {
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("cluster client certificate is not ECDSA")
	}
	sum := sha256.Sum256([]byte(canonical))
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		return fmt.Errorf("cluster request signature mismatch")
	}
	return nil
}

func parseECPrivateKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in client key")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("client key is not ECDSA")
	}
	return key, nil
}

// CanonicalClusterRequest is the signed bytes for rotate and revoke-notify.
func CanonicalClusterRequest(method, path string, body []byte) string {
	sum := sha256.Sum256(body)
	return method + "\n" + path + "\n" + fmt.Sprintf("%x", sum[:])
}
