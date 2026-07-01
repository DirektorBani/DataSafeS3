package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

func (s *Store) FieldEnc() *fieldenc.Service {
	return s.fieldenc
}

// EncryptionRegistryCount returns rows in encryption_key_registry (0 when table empty).
func (s *Store) EncryptionRegistryCount() int {
	if s == nil || s.pool == nil {
		return 0
	}
	var n int
	_ = s.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM encryption_key_registry`).Scan(&n)
	return n
}

func (s *Store) fieldPrepare(path, val string) (string, error) {
	if s.fieldenc == nil || val == "" {
		return val, nil
	}
	out, _, err := s.fieldenc.RewrapIfNeeded(path, val)
	return out, err
}

func (s *Store) fieldDecrypt(path, val string) (string, error) {
	if s.fieldenc == nil {
		return val, nil
	}
	return s.fieldenc.Decrypt(path, val)
}
