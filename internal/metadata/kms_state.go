package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

// KMSState is the shared fingerprint and legacy-SSE flag. See ADR-0006.
type KMSState struct {
	Fingerprint      string `json:"fingerprint,omitempty"`
	LegacySSEObjects bool   `json:"legacy_sse_objects,omitempty"`
	Initialized      bool   `json:"initialized,omitempty"`
}

// KMSKeyRecord is one wrapping key. Material stays in the provider, not here.
type KMSKeyRecord struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	Status     string    `json:"status"`
	Version    int       `json:"key_version"`
	Generation int64     `json:"generation"`
	TenantID   string    `json:"tenant_id,omitempty"`
	BucketKey  string    `json:"bucket_key,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// BucketEncryption is the bucket default, stored beside the bucket row.
type BucketEncryption struct {
	StorageKey string `json:"storage_key"`
	Algorithm  string `json:"algorithm,omitempty"`
	KeyID      string `json:"key_id,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Status     string `json:"status,omitempty"`
}

// NodeHeartbeat is one storage-server version seen by the cluster.
type NodeHeartbeat struct {
	NodeID  string    `json:"node_id"`
	Version string    `json:"version"`
	SeenAt  time.Time `json:"seen_at"`
}

func (s *Store) kmsBucket(tx *bolt.Tx, name string) (*bolt.Bucket, error) {
	b := tx.Bucket([]byte(name))
	if b != nil {
		return b, nil
	}
	return tx.CreateBucket([]byte(name))
}

func (s *Store) GetKMSState() (KMSState, error) {
	var st KMSState
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("kms_state"))
		if b == nil {
			return nil
		}
		data := b.Get([]byte("local"))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &st)
	})
	return st, err
}

// InitKMSState inserts the row once. A second caller gets the stored row.
func (s *Store) InitKMSState(st KMSState) (KMSState, error) {
	var stored KMSState
	err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.kmsBucket(tx, "kms_state")
		if err != nil {
			return err
		}
		if cur := b.Get([]byte("local")); cur != nil {
			return json.Unmarshal(cur, &stored)
		}
		st.Initialized = true
		data, err := json.Marshal(st)
		if err != nil {
			return err
		}
		if err := b.Put([]byte("local"), data); err != nil {
			return err
		}
		stored = st
		return nil
	})
	return stored, err
}

func (s *Store) PutKMSState(st KMSState) error {
	st.Initialized = true
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.kmsBucket(tx, "kms_state")
		if err != nil {
			return err
		}
		data, err := json.Marshal(st)
		if err != nil {
			return err
		}
		return b.Put([]byte("local"), data)
	})
}

func (s *Store) PutKMSKey(rec KMSKeyRecord) error {
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.kmsBucket(tx, "kms_keys")
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetKMSKey(id string) (KMSKeyRecord, error) {
	var rec KMSKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("kms_keys"))
		if b == nil {
			return ErrNotFound
		}
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) PutBucketEncryption(storageKey string, enc BucketEncryption) error {
	rec, err := s.GetBucketByKey(storageKey)
	if err != nil {
		return err
	}
	rec.DefaultEncryption = enc.Algorithm
	rec.KMSMasterKeyID = enc.KeyID
	rec.EncryptionProvider = enc.Provider
	rec.KMSKeyStatus = enc.Status
	return s.UpdateBucket(rec)
}

func (s *Store) GetBucketEncryption(storageKey string) (BucketEncryption, error) {
	rec, err := s.GetBucketByKey(storageKey)
	if err != nil {
		return BucketEncryption{}, err
	}
	return BucketEncryption{
		StorageKey: storageKey,
		Algorithm:  rec.DefaultEncryption,
		KeyID:      rec.KMSMasterKeyID,
		Provider:   rec.EncryptionProvider,
		Status:     rec.KMSKeyStatus,
	}, nil
}

func (s *Store) DeleteBucketEncryption(storageKey string) error {
	return s.PutBucketEncryption(storageKey, BucketEncryption{StorageKey: storageKey})
}

func (s *Store) UpsertNodeHeartbeat(nodeID, version string, at time.Time) error {
	rec := NodeHeartbeat{NodeID: nodeID, Version: version, SeenAt: at.UTC()}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.kmsBucket(tx, "cluster_node_heartbeats")
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(nodeID), data)
	})
}

func (s *Store) ListNodeHeartbeats(since time.Time) ([]NodeHeartbeat, error) {
	var out []NodeHeartbeat
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("cluster_node_heartbeats"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec NodeHeartbeat
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if !since.IsZero() && rec.SeenAt.Before(since) {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}
