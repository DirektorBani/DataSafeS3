package metadata

import (
	"encoding/json"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	bolt "go.etcd.io/bbolt"
)

const boltKEKRegistryKey = "encryption_key_registry"

func (s *Store) FieldEnc() *fieldenc.Service {
	return s.fieldenc
}

// EncryptionRegistryCount returns KEK registry entries stored in Bolt config bucket.
func (s *Store) EncryptionRegistryCount() int {
	if s == nil || s.db == nil {
		return 0
	}
	var n int
	_ = s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("config")).Get([]byte(boltKEKRegistryKey))
		if len(data) == 0 {
			return nil
		}
		var entries []struct {
			KEKID string `json:"kek_id"`
		}
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil
		}
		n = len(entries)
		return nil
	})
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
