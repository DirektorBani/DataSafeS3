package metadata

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	"golang.org/x/crypto/hkdf"
)

// kmsSealPrefix marks a Vault secret sealed with the local key.
// Field encryption (enc:v1) is used instead when that feature is on.
const kmsSealPrefix = "kse:v1:"

// SetKMSSealMaterial installs the key that seals Vault secret_id and token
// even when STORAGE_FIELD_ENCRYPTION_ENABLED is false.
// The material is the server key when one is set, otherwise the JWT secret.
func SetKMSSealMaterial(store MetadataStore, material string) {
	type setter interface {
		SetKMSSealMaterial(string)
	}
	if s, ok := store.(setter); ok {
		s.SetKMSSealMaterial(material)
	}
}

func (s *Store) SetKMSSealMaterial(material string) {
	if s == nil {
		return
	}
	s.kmsSealKey = deriveKMSSealKey(material)
}

func (s *Store) kmsKey() []byte {
	if s != nil && len(s.kmsSealKey) == 32 {
		return s.kmsSealKey
	}
	return deriveKMSSealKey(defaultKMSSealMaterial())
}

func defaultKMSSealMaterial() string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_SSE_MASTER_KEY")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_JWT_SECRET")); v != "" {
		return v
	}
	return "datasafe-kms-at-rest"
}

// DeriveKMSSealKey returns the 32-byte key for Vault secrets at rest.
func DeriveKMSSealKey(material string) []byte {
	return deriveKMSSealKey(material)
}

func deriveKMSSealKey(material string) []byte {
	material = strings.TrimSpace(material)
	if material == "" {
		material = defaultKMSSealMaterial()
	}
	r := hkdf.New(sha256.New, []byte(material), []byte("datasafe-kms-at-rest"), []byte("v1"))
	key := make([]byte, 32)
	_, _ = io.ReadFull(r, key)
	return key
}

func sealKMSValue(key []byte, path, plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, fieldencWirePrefix()) || strings.HasPrefix(plain, kmsSealPrefix) {
		return plain, nil
	}
	if len(key) != 32 {
		return "", errors.New("kms seal key is missing")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), []byte(path))
	return kmsSealPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

func openKMSValue(key []byte, path, stored string) (string, error) {
	if !strings.HasPrefix(stored, kmsSealPrefix) {
		return stored, nil
	}
	if len(key) != 32 {
		return "", errors.New("kms seal key is missing")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, kmsSealPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("kms seal value is truncated")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, []byte(path))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func fieldencWirePrefix() string {
	return "enc:v1:"
}

func sealKMSConfig(key []byte, cfg VaultKMSConfig) (VaultKMSConfig, error) {
	var err error
	if cfg.SecretID, err = sealKMSValue(key, fieldenc.PathKMSVaultSecretID, cfg.SecretID); err != nil {
		return cfg, err
	}
	if cfg.Token, err = sealKMSValue(key, fieldenc.PathKMSVaultToken, cfg.Token); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func openKMSConfig(key []byte, cfg VaultKMSConfig) (VaultKMSConfig, error) {
	var err error
	if cfg.SecretID, err = openKMSValue(key, fieldenc.PathKMSVaultSecretID, cfg.SecretID); err != nil {
		return cfg, err
	}
	if cfg.Token, err = openKMSValue(key, fieldenc.PathKMSVaultToken, cfg.Token); err != nil {
		return cfg, err
	}
	return cfg, nil
}
