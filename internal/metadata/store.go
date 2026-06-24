package metadata

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrBucketExists    = errors.New("bucket already exists")
	ErrBucketNotEmpty = errors.New("bucket not empty")
	ErrQuotaExceeded  = errors.New("quota exceeded")
)

type LifecycleRule struct {
	ID             string `json:"id"`
	Name           string `json:"name,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	Action         string `json:"action,omitempty"` // expire, abort_multipart, expire_noncurrent
	ExpirationDays int    `json:"expiration_days"`
	Enabled        bool   `json:"enabled"`
}

const (
	LifecycleExpire            = "expire"
	LifecycleAbortMultipart    = "abort_multipart"
	LifecycleExpireNoncurrent  = "expire_noncurrent"
)

type BucketRecord struct {
	StorageKey       string          `json:"storage_key,omitempty"`
	Name             string          `json:"name"`
	CreatedAt        time.Time       `json:"created_at"`
	Owner            string          `json:"owner"`
	OwnerID          string          `json:"owner_id,omitempty"`
	TeamID           string          `json:"team_id,omitempty"`
	Policy           string          `json:"policy,omitempty"`
	LifecycleRules   []LifecycleRule `json:"lifecycle_rules,omitempty"`
	Description      string          `json:"description,omitempty"`
	Versioning           bool `json:"versioning_enabled"`
	VersioningSuspended  bool `json:"versioning_suspended,omitempty"`
	ObjectLock           bool   `json:"object_lock_enabled"`
	RetentionDays        int    `json:"retention_days,omitempty"`
	RetentionMode        string `json:"retention_mode,omitempty"` // GOVERNANCE | COMPLIANCE
	StorageClass         string            `json:"storage_class,omitempty"`
	TenantID             string            `json:"tenant_id,omitempty"`
	Visibility       string          `json:"visibility,omitempty"` // private, public-read
	MaxSizeBytes     int64             `json:"max_size_bytes,omitempty"`
	MaxObjects       int64             `json:"max_objects,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
}

type ObjectRecord struct {
	Bucket            string            `json:"bucket"`
	Key               string            `json:"key"`
	Size              int64             `json:"size"`
	ETag              string            `json:"etag"`
	ContentType       string            `json:"content_type"`
	VersionID         string            `json:"version_id"`
	LastModified      time.Time         `json:"last_modified"`
	ScheduledDeleteAt *time.Time        `json:"scheduled_delete_at,omitempty"`
	IsDeleteMarker    bool              `json:"is_delete_marker,omitempty"`
	LegalHold         bool              `json:"legal_hold,omitempty"`
	RetentionUntil    *time.Time        `json:"retention_until,omitempty"`
	StorageClass      string            `json:"storage_class,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	CreatedAt         time.Time         `json:"created_at,omitempty"`
}

type MultipartRecord struct {
	UploadID  string    `json:"upload_id"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key"`
	Initiated time.Time `json:"initiated"`
}

type AccessKeyRecord struct {
	AccessKey    string     `json:"access_key"`
	SecretKey    string     `json:"secret_key"`
	SessionToken string     `json:"session_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Label        string     `json:"label"`
	OwnerID      string     `json:"owner_id,omitempty"`
	Owner        string     `json:"owner,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Store struct {
	db *bolt.DB
}

func boltOpen(path string) (*bolt.DB, error) {
	return bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{
			"buckets", "bucket_scope_index", "bucket_access_grants", "bucket_prefix_access_grants",
			"objects", "object_latest", "multipart", "access_keys",
			"users", "user_index", "activity", "usage_counters", "usage_snapshots",
			"config", "trash", "api_tokens", "webhooks", "favorites", "webhook_deliveries",
			"user_notifications", "recent_items",
		} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		if err := s.migrateBucketScopeIndex(tx); err != nil {
			return err
		}
		return s.initEnterpriseBuckets(tx)
	})
}

func (s *Store) migrateBucketScopeIndex(tx *bolt.Tx) error {
	idx, err := tx.CreateBucketIfNotExists([]byte("bucket_scope_index"))
	if err != nil {
		return err
	}
	buckets := tx.Bucket([]byte("buckets"))
	return buckets.ForEach(func(k, v []byte) error {
		var rec BucketRecord
		if err := json.Unmarshal(v, &rec); err != nil {
			return err
		}
		if rec.StorageKey == "" {
			rec.StorageKey = string(k)
		}
		scope := BucketScopeForRecord(rec.TenantID, rec.OwnerID)
		if idx.Get([]byte(ScopeIndexKey(scope, rec.Name))) == nil {
			if err := idx.Put([]byte(ScopeIndexKey(scope, rec.Name)), []byte(rec.StorageKey)); err != nil {
				return err
			}
		}
		if string(k) != rec.StorageKey {
			data, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			if err := buckets.Put([]byte(rec.StorageKey), data); err != nil {
				return err
			}
			return buckets.Delete(k)
		}
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return buckets.Put(k, updated)
	})
}

func (s *Store) PutBucket(rec BucketRecord) error {
	scope := BucketScopeForRecord(rec.TenantID, rec.OwnerID)
	if rec.StorageKey == "" {
		rec.StorageKey = MakeStorageKey(scope, rec.Name)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		idx := tx.Bucket([]byte("bucket_scope_index"))
		if idx.Get([]byte(ScopeIndexKey(scope, rec.Name))) != nil {
			return ErrBucketExists
		}
		b := tx.Bucket([]byte("buckets"))
		if b.Get([]byte(rec.StorageKey)) != nil {
			return ErrBucketExists
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(rec.StorageKey), data); err != nil {
			return err
		}
		return idx.Put([]byte(ScopeIndexKey(scope, rec.Name)), []byte(rec.StorageKey))
	})
}

func (s *Store) GetBucketByKey(storageKey string) (BucketRecord, error) {
	var rec BucketRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("buckets")).Get([]byte(storageKey))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) GetBucket(name string) (BucketRecord, error) {
	rec, err := s.GetBucketByKey(name)
	if err == nil {
		return rec, nil
	}
	all, err := s.ListBuckets()
	if err != nil {
		return rec, err
	}
	var matches []BucketRecord
	for _, b := range all {
		if b.Name == name {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return BucketRecord{}, ErrNotFound
}

func (s *Store) ResolveBucket(scope BucketScope, name string) (BucketRecord, error) {
	var rec BucketRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket([]byte("bucket_scope_index"))
		if key := idx.Get([]byte(ScopeIndexKey(scope, name))); key != nil {
			data := tx.Bucket([]byte("buckets")).Get(key)
			if data == nil {
				return ErrNotFound
			}
			return json.Unmarshal(data, &rec)
		}
		if scope.Kind != ScopeOwner {
			return ErrNotFound
		}
		// Legacy: storage_key == logical name, must belong to this owner scope.
		data := tx.Bucket([]byte("buckets")).Get([]byte(name))
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		if rec.LegacyBucket() && rec.OwnerID == scope.OwnerID {
			return nil
		}
		return ErrNotFound
	})
	return rec, err
}

func (s *Store) DeleteBucket(storageKey string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("buckets"))
		data := b.Get([]byte(storageKey))
		if data == nil {
			return ErrNotFound
		}
		var rec BucketRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		scope := BucketScopeForRecord(rec.TenantID, rec.OwnerID)
		_ = tx.Bucket([]byte("bucket_scope_index")).Delete([]byte(ScopeIndexKey(scope, rec.Name)))
		prefix := []byte(storageKey + "\x00")
		latest := tx.Bucket([]byte("object_latest"))
		if c := latest.Cursor(); c != nil {
			for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
				return ErrBucketNotEmpty
			}
		}
		c := tx.Bucket([]byte("objects")).Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			return ErrBucketNotEmpty
		}
		return b.Delete([]byte(storageKey))
	})
}

func (s *Store) ListBuckets() ([]BucketRecord, error) {
	var out []BucketRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("buckets")).ForEach(func(k, v []byte) error {
			var rec BucketRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func objectKey(bucket, key string) []byte {
	return []byte(bucket + "\x00" + key)
}

// Object CRUD with optional versioning lives in objects.go.

func splitObjectKey(k []byte) [2]string {
	s := string(k)
	i := 0
	for i < len(s) && s[i] != '\x00' {
		i++
	}
	if i >= len(s) {
		return [2]string{s, ""}
	}
	return [2]string{s[:i], s[i+1:]}
}

func (s *Store) PutMultipart(rec MultipartRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("multipart")).Put([]byte(rec.UploadID), data)
	})
}

func (s *Store) GetMultipart(uploadID string) (MultipartRecord, error) {
	var rec MultipartRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("multipart")).Get([]byte(uploadID))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) DeleteMultipart(uploadID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("multipart"))
		if b.Get([]byte(uploadID)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(uploadID))
	})
}

func (s *Store) ListMultipart(bucket string) ([]MultipartRecord, error) {
	var out []MultipartRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("multipart")).ForEach(func(_, v []byte) error {
			var rec MultipartRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if bucket == "" || rec.Bucket == bucket {
				out = append(out, rec)
			}
			return nil
		})
	})
	return out, err
}

func (s *Store) PutAccessKey(rec AccessKeyRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("access_keys")).Put([]byte(rec.AccessKey), data)
	})
}

func (s *Store) GetAccessKey(accessKey string) (AccessKeyRecord, error) {
	var rec AccessKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("access_keys")).Get([]byte(accessKey))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListAccessKeys() ([]AccessKeyRecord, error) {
	var out []AccessKeyRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("access_keys")).ForEach(func(k, v []byte) error {
			var rec AccessKeyRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteAccessKey(accessKey string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("access_keys"))
		if b.Get([]byte(accessKey)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(accessKey))
	})
}

func (s *Store) SetBucketPolicy(storageKey, policy string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("buckets"))
		data := b.Get([]byte(storageKey))
		if data == nil {
			return ErrNotFound
		}
		var rec BucketRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Policy = policy
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(storageKey), updated)
	})
}

func (s *Store) SetBucketLifecycle(storageKey string, rules []LifecycleRule) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("buckets"))
		data := b.Get([]byte(storageKey))
		if data == nil {
			return ErrNotFound
		}
		var rec BucketRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.LifecycleRules = rules
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(storageKey), updated)
	})
}

func (s *Store) TotalObjectBytes() (int64, error) {
	return s.totalLatestObjectBytes()
}
