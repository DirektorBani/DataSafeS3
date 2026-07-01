package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

const boltEncryptionRegistryKey = "encryption_key_registry"

func (s *Store) ListEncryptionKeys(ctx context.Context) ([]EncryptionKeyRecord, error) {
	var out []EncryptionKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("config")).Get([]byte(boltEncryptionRegistryKey))
		if len(data) == 0 {
			return nil
		}
		return json.Unmarshal(data, &out)
	})
	return out, err
}

func (s *Store) GetEncryptionKey(ctx context.Context, kekID string) (EncryptionKeyRecord, error) {
	keys, err := s.ListEncryptionKeys(ctx)
	if err != nil {
		return EncryptionKeyRecord{}, err
	}
	for _, k := range keys {
		if k.KEKID == kekID {
			return k, nil
		}
	}
	return EncryptionKeyRecord{}, ErrNotFound
}

func (s *Store) GetActiveEncryptionKey(ctx context.Context) (EncryptionKeyRecord, error) {
	keys, err := s.ListEncryptionKeys(ctx)
	if err != nil {
		return EncryptionKeyRecord{}, err
	}
	for _, k := range keys {
		if k.IsActive && k.RetiredAt == nil {
			return k, nil
		}
	}
	return EncryptionKeyRecord{}, ErrNotFound
}

func (s *Store) InsertEncryptionKey(ctx context.Context, rec EncryptionKeyRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("config"))
		var keys []EncryptionKeyRecord
		if data := b.Get([]byte(boltEncryptionRegistryKey)); len(data) > 0 {
			if err := json.Unmarshal(data, &keys); err != nil {
				return err
			}
		}
		for _, k := range keys {
			if k.KEKID == rec.KEKID {
				return nil
			}
		}
		if rec.CreatedAt.IsZero() {
			rec.CreatedAt = time.Now().UTC()
		}
		if rec.Algorithm == "" {
			rec.Algorithm = EncryptionAlgorithmX25519
		}
		keys = append(keys, rec)
		data, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		return b.Put([]byte(boltEncryptionRegistryKey), data)
	})
}

func (s *Store) SetEncryptionKeyActive(ctx context.Context, kekID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("config"))
		data := b.Get([]byte(boltEncryptionRegistryKey))
		if len(data) == 0 {
			return ErrNotFound
		}
		var keys []EncryptionKeyRecord
		if err := json.Unmarshal(data, &keys); err != nil {
			return err
		}
		found := false
		now := time.Now().UTC()
		for i := range keys {
			if keys[i].KEKID == kekID {
				keys[i].IsActive = true
				found = true
			} else if keys[i].IsActive {
				keys[i].IsActive = false
				if keys[i].RotatedAt == nil {
					keys[i].RotatedAt = &now
				}
			}
		}
		if !found {
			return ErrNotFound
		}
		out, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		return b.Put([]byte(boltEncryptionRegistryKey), out)
	})
}

var _ EncryptionKeyRegistry = (*Store)(nil)

// ErrEncryptionRegistryEmpty is returned when bootstrap expects an active key but registry is empty.
var ErrEncryptionRegistryEmpty = errors.New("encryption key registry is empty")
