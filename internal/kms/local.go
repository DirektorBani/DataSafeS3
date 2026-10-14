package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
)

const (
	// SSEAlgorithmAES256 is SSE-S3 with the server key.
	SSEAlgorithmAES256 = "AES256"
	// SSEAlgorithmKMS is SSE-KMS. E01 wraps the DEK with the local KEK.
	SSEAlgorithmKMS = "aws:kms"
	// ProviderLocal is the on-box KEK from STORAGE_SSE_MASTER_KEY.
	ProviderLocal = "local"
	// ProviderVault is reserved for E02.
	ProviderVault = "vault-transit"

	kekSalt = "datasafe-sse-kek"
	dekAAD  = "datasafe-dek"
)

// ErrNotConfigured means STORAGE_SSE_MASTER_KEY is empty.
var ErrNotConfigured = errors.New("STORAGE_SSE_MASTER_KEY is not configured")

// ErrKeyDestroyed means the key row was destroyed and the DEK must not be unwrapped.
var ErrKeyDestroyed = errors.New("key destroyed")

// Local is the process KEK. It is safe for concurrent use.
type Local struct {
	raw     string
	derived [32]byte
}

// NewLocal returns nil when master is empty. The derived key matches 1.5.0:
// SHA-256 of the master string, used as AES-256.
func NewLocal(master string) (*Local, error) {
	if master == "" {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(master))
	return &Local{raw: master, derived: sum}, nil
}

// Fingerprint is a non-secret id of the derived key. Nodes compare this at startup.
func Fingerprint(master string) string {
	if master == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(master))
	out := sha256.Sum256(sum[:])
	return hex.EncodeToString(out[:])
}

// Fingerprint of this process key.
func (l *Local) Fingerprint() string {
	if l == nil {
		return ""
	}
	return Fingerprint(l.raw)
}

func (l *Local) Name() string { return ProviderLocal }

func (l *Local) kek(version int) ([]byte, error) {
	if l == nil {
		return nil, ErrNotConfigured
	}
	if version <= 0 {
		out := make([]byte, 32)
		copy(out, l.derived[:])
		return out, nil
	}
	out, err := hkdf.Key(sha256.New, l.derived[:], []byte(kekSalt), "v"+strconv.Itoa(version), 32)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func aeadFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Wrap seals dek with the KEK at version (1 when version < 1).
func (l *Local) Wrap(_ context.Context, _ string, dek []byte) ([]byte, int, error) {
	return l.WrapVersion(dek, 1)
}

// WrapVersion seals dek with a specific KEK version.
func (l *Local) WrapVersion(dek []byte, version int) ([]byte, int, error) {
	if version < 1 {
		version = 1
	}
	kek, err := l.kek(version)
	if err != nil {
		return nil, 0, err
	}
	aead, err := aeadFor(kek)
	if err != nil {
		return nil, 0, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, 0, err
	}
	return aead.Seal(nonce, nonce, dek, []byte(dekAAD)), version, nil
}

// Unwrap opens a wrapped DEK.
func (l *Local) Unwrap(_ context.Context, _ string, version int, wrapped []byte) ([]byte, error) {
	if version < 1 {
		version = 1
	}
	kek, err := l.kek(version)
	if err != nil {
		return nil, err
	}
	aead, err := aeadFor(kek)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < aead.NonceSize() {
		return nil, errors.New("wrapped dek too short")
	}
	nonce := wrapped[:aead.NonceSize()]
	return aead.Open(nil, nonce, wrapped[aead.NonceSize():], []byte(dekAAD))
}

// Rewrap unwraps with version and wraps with version+1. Callers persist the new version.
func (l *Local) Rewrap(ctx context.Context, keyID string, version int, wrapped []byte) ([]byte, int, error) {
	dek, err := l.Unwrap(ctx, keyID, version, wrapped)
	if err != nil {
		return nil, 0, err
	}
	out, ver, err := l.WrapVersion(dek, version+1)
	for i := range dek {
		dek[i] = 0
	}
	return out, ver, err
}

// Rotate reports the next version. The key row is updated by the caller.
func (l *Local) Rotate(_ context.Context, _ string) (int, error) {
	if l == nil {
		return 0, ErrNotConfigured
	}
	return 0, errors.New("local rotate is applied by the key row")
}

// Destroy is recorded on the key row. The provider has no remote key to delete.
func (l *Local) Destroy(context.Context, string) error {
	if l == nil {
		return ErrNotConfigured
	}
	return nil
}

// Health is nil when the process key is configured.
func (l *Local) Health(context.Context) error {
	if l == nil {
		return ErrNotConfigured
	}
	return nil
}

// EncryptFormat0 seals plaintext the way 1.5.0 did: one GCM blob, nonce prefixed, KEK version 0.
func (l *Local) EncryptFormat0(plaintext []byte) ([]byte, error) {
	kek, err := l.kek(0)
	if err != nil {
		return nil, err
	}
	aead, err := aeadFor(kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptFormat0 opens a 1.5.0 blob.
func (l *Local) DecryptFormat0(ciphertext []byte) ([]byte, error) {
	kek, err := l.kek(0)
	if err != nil {
		return nil, err
	}
	aead, err := aeadFor(kek)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:aead.NonceSize()]
	return aead.Open(nil, nonce, ciphertext[aead.NonceSize():], nil)
}

// NewDEK returns 32 random bytes.
func NewDEK() ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	return dek, nil
}
