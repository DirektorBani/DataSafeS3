package metadata

import (
	"context"
	"time"
)

const EncryptionAlgorithmX25519 = "x25519-aes256-gcm"

// EncryptionKeyRecord holds public KEK metadata for field encryption (private keys stay in env).
type EncryptionKeyRecord struct {
	KEKID     string     `json:"kek_id"`
	Algorithm string     `json:"algorithm"`
	PublicKey []byte     `json:"public_key"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
	RetiredAt *time.Time `json:"retired_at,omitempty"`
}

// EncryptionKeyRegistry provides CRUD for the KEK metadata registry.
type EncryptionKeyRegistry interface {
	ListEncryptionKeys(ctx context.Context) ([]EncryptionKeyRecord, error)
	GetEncryptionKey(ctx context.Context, kekID string) (EncryptionKeyRecord, error)
	GetActiveEncryptionKey(ctx context.Context) (EncryptionKeyRecord, error)
	InsertEncryptionKey(ctx context.Context, rec EncryptionKeyRecord) error
	SetEncryptionKeyActive(ctx context.Context, kekID string) error
}
