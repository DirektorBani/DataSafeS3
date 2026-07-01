package fieldenc

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// StatusBlock is returned by GET /api/v1/settings/security-status (field_encryption).
type StatusBlock struct {
	Enabled                       bool   `json:"enabled"`
	ActiveKEKID                   string `json:"active_kek_id,omitempty"`
	RegistryCount                 int    `json:"registry_count"`
	LegacyPlaintextFieldsEstimate int    `json:"legacy_plaintext_fields_estimate"`
}

// Service provides opt-in envelope encryption for metadata secret fields.
type Service struct {
	enabled     bool
	activeKekID string
	privateKeys map[string]*ecdh.PrivateKey
	activePriv  *ecdh.PrivateKey
	retired     map[string]bool
	legacyPlain int
}

// Enabled reports whether encrypt-on-write is active.
func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

// ActiveKEKID returns the configured active key id (may be empty when disabled).
func (s *Service) ActiveKEKID() string {
	if s == nil {
		return ""
	}
	return s.activeKekID
}

// ActivePublicKey returns the raw 32-byte X25519 public key for the active KEK.
func (s *Service) ActivePublicKey() []byte {
	if s == nil || s.activePriv == nil {
		return nil
	}
	return bytes.Clone(s.activePriv.PublicKey().Bytes())
}

// Status returns non-secret status for the security-status API.
func (s *Service) Status(registryCount int) StatusBlock {
	if s == nil {
		return StatusBlock{}
	}
	return StatusBlock{
		Enabled:                       s.enabled,
		ActiveKEKID:                   s.activeKekID,
		RegistryCount:                 registryCount,
		LegacyPlaintextFieldsEstimate: s.legacyPlain,
	}
}

// SetLegacyPlaintextEstimate sets the optional legacy-plaintext counter (store scan).
func (s *Service) SetLegacyPlaintextEstimate(n int) {
	if s != nil {
		s.legacyPlain = n
	}
}

// IsEncrypted reports whether stored uses the enc:v1 wire prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, wirePrefix)
}

// Encrypt seals plaintext with the active KEK when enabled; otherwise returns plaintext.
func (s *Service) Encrypt(fieldPath, plaintext string) (string, error) {
	if s == nil || !s.enabled || plaintext == "" {
		return plaintext, nil
	}
	if s.activePriv == nil {
		return "", fmt.Errorf("fieldenc: no active private key")
	}
	return encryptValue(s.activePriv, s.activeKekID, fieldPath, plaintext)
}

// Decrypt unwraps enc:v1 values; plaintext values pass through unchanged.
func (s *Service) Decrypt(fieldPath, stored string) (string, error) {
	if stored == "" || !IsEncrypted(stored) {
		return stored, nil
	}
	if s == nil || len(s.privateKeys) == 0 {
		return "", fmt.Errorf("fieldenc: no private keys configured for decrypt")
	}
	kekID, err := kekIDFromWire(stored)
	if err != nil {
		return "", err
	}
	if s.retired != nil && s.retired[kekID] {
		return "", fmt.Errorf("fieldenc: kek_id %q is retired", kekID)
	}
	return decryptValue(s.privateKeys, fieldPath, stored)
}

// RewrapIfNeeded encrypts plaintext or re-encrypts with the active KEK when an older kek_id is used.
// The bool is true when the returned value differs from stored.
func (s *Service) RewrapIfNeeded(fieldPath, stored string) (string, bool, error) {
	if s == nil || !s.enabled || stored == "" {
		return stored, false, nil
	}
	if !IsEncrypted(stored) {
		enc, err := s.Encrypt(fieldPath, stored)
		return enc, true, err
	}
	kekID, err := kekIDFromWire(stored)
	if err != nil {
		return stored, false, err
	}
	if kekID == s.activeKekID {
		return stored, false, nil
	}
	plain, err := s.Decrypt(fieldPath, stored)
	if err != nil {
		return stored, false, err
	}
	enc, err := s.Encrypt(fieldPath, plain)
	return enc, true, err
}

// FromEnv builds a Service from process environment (disabled when unset/false).
func FromEnv() (*Service, error) {
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("STORAGE_FIELD_ENCRYPTION_ENABLED")), "true")
	if !enabled {
		return &Service{enabled: false, privateKeys: map[string]*ecdh.PrivateKey{}}, nil
	}

	activeID := strings.TrimSpace(os.Getenv("STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID"))
	if activeID == "" {
		return nil, fmt.Errorf("fieldenc: STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID required when enabled")
	}

	keys, err := loadPrivateKeysFromEnv()
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("fieldenc: STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY or _KEK_PRIVATE_KEYS required when enabled")
	}
	active, ok := keys[activeID]
	if !ok {
		return nil, fmt.Errorf("fieldenc: active kek_id %q has no private key in env", activeID)
	}
	return &Service{
		enabled:     true,
		activeKekID: activeID,
		privateKeys: keys,
		activePriv:  active,
		retired:     make(map[string]bool),
	}, nil
}

// NewForTest constructs an enabled service for unit/integration tests.
func NewForTest(kekID string, privateSeed []byte, enabled bool) (*Service, error) {
	curve := ecdh.X25519()
	priv, err := curve.NewPrivateKey(privateSeed)
	if err != nil {
		return nil, err
	}
	keys := map[string]*ecdh.PrivateKey{kekID: priv}
	return &Service{
		enabled:     enabled,
		activeKekID: kekID,
		privateKeys: keys,
		activePriv:  priv,
	}, nil
}

func loadPrivateKeysFromEnv() (map[string]*ecdh.PrivateKey, error) {
	out := map[string]*ecdh.PrivateKey{}
	if multi := strings.TrimSpace(os.Getenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS")); multi != "" {
		var raw map[string]string
		if err := json.Unmarshal([]byte(multi), &raw); err != nil {
			return nil, fmt.Errorf("fieldenc: parse STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS: %w", err)
		}
		for id, b64 := range raw {
			priv, err := decodePrivateKey(b64)
			if err != nil {
				return nil, fmt.Errorf("fieldenc: kek %q: %w", id, err)
			}
			out[id] = priv
		}
	}
	if single := strings.TrimSpace(os.Getenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY")); single != "" {
		activeID := strings.TrimSpace(os.Getenv("STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID"))
		if activeID == "" {
			return nil, fmt.Errorf("fieldenc: STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID required with single private key")
		}
		priv, err := decodePrivateKey(single)
		if err != nil {
			return nil, err
		}
		out[activeID] = priv
	}
	return out, nil
}

func decodePrivateKey(b64 string) (*ecdh.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("fieldenc: decode private key: %w", err)
	}
	curve := ecdh.X25519()
	priv, err := curve.NewPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("fieldenc: invalid X25519 private key: %w", err)
	}
	return priv, nil
}

// NewFromEnv is an alias for FromEnv (startup wiring).
func NewFromEnv() (*Service, error) {
	return FromEnv()
}

// MarkRetiredKEKs records kek_ids that must not decrypt (registry retired_at set).
func (s *Service) MarkRetiredKEKs(retired map[string]bool) {
	if s == nil {
		return
	}
	s.retired = retired
}
