package metadata

import (
	"encoding/json"
	"sort"
	"strings"

	bolt "go.etcd.io/bbolt"
)

func versionObjectKey(bucket, key, versionID string) []byte {
	return []byte(bucket + "\x00" + key + "\x00" + versionID)
}

func latestIndexKey(bucket, key string) []byte {
	return []byte(bucket + "\x00" + key)
}

func splitVersionObjectKey(k []byte) (bucket, key, versionID string) {
	parts := strings.Split(string(k), "\x00")
	if len(parts) < 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

func (s *Store) normalizeObjectBucket(bucket string) string {
	if rec, err := s.GetBucket(bucket); err == nil {
		return rec.EffectiveStorageKey()
	}
	return bucket
}

// PutObject stores or overwrites the latest object when versioning is disabled.
func (s *Store) PutObject(rec ObjectRecord) error {
	return s.putObject(rec, false)
}

// PutObjectVersioned appends a new version and updates the latest pointer.
func (s *Store) PutObjectVersioned(rec ObjectRecord) error {
	return s.putObject(rec, true)
}

func (s *Store) putObject(rec ObjectRecord, versioned bool) error {
	rec.Bucket = s.normalizeObjectBucket(rec.Bucket)
	return s.db.Update(func(tx *bolt.Tx) error {
		objects := tx.Bucket([]byte("objects"))
		latest := tx.Bucket([]byte("object_latest"))
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if versioned {
			if rec.VersionID == "" {
				return ErrNotFound
			}
			if err := objects.Put(versionObjectKey(rec.Bucket, rec.Key, rec.VersionID), data); err != nil {
				return err
			}
			return latest.Put(latestIndexKey(rec.Bucket, rec.Key), []byte(rec.VersionID))
		}
		// Non-versioned: remove version index and legacy entries, store flat key.
		_ = s.deleteAllVersionsTx(tx, rec.Bucket, rec.Key)
		return objects.Put(objectKey(rec.Bucket, rec.Key), data)
	})
}

func (s *Store) deleteAllVersionsTx(tx *bolt.Tx, bucket, key string) error {
	objects := tx.Bucket([]byte("objects"))
	latest := tx.Bucket([]byte("object_latest"))
	prefix := []byte(bucket + "\x00" + key + "\x00")
	c := objects.Cursor()
	for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
		if err := objects.Delete(k); err != nil {
			return err
		}
	}
	_ = latest.Delete(latestIndexKey(bucket, key))
	_ = objects.Delete(objectKey(bucket, key))
	return nil
}

// GetObject returns the latest object record for a key.
func (s *Store) GetObject(bucket, key string) (ObjectRecord, error) {
	return s.GetObjectVersion(bucket, key, "")
}

// GetObjectVersion returns a specific version or the latest when versionID is empty.
func (s *Store) GetObjectVersion(bucket, key, versionID string) (ObjectRecord, error) {
	bucket = s.normalizeObjectBucket(bucket)
	var rec ObjectRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		objects := tx.Bucket([]byte("objects"))
		if versionID == "" {
			if vid := tx.Bucket([]byte("object_latest")).Get(latestIndexKey(bucket, key)); vid != nil {
				versionID = string(vid)
			}
		}
		var data []byte
		if versionID != "" {
			data = objects.Get(versionObjectKey(bucket, key, versionID))
		}
		if data == nil {
			data = objects.Get(objectKey(bucket, key))
		}
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		if versionID == "" && rec.IsDeleteMarker {
			return ErrNotFound
		}
		return nil
	})
	return rec, err
}

// DeleteObject removes the latest object when versioning is disabled.
func (s *Store) DeleteObject(bucket, key string) error {
	bucket = s.normalizeObjectBucket(bucket)
	return s.DeleteObjectVersion(bucket, key, "", false)
}

// DeleteObjectVersion deletes a specific version or the latest when versionID is empty.
func (s *Store) DeleteObjectVersion(bucket, key, versionID string, versioningEnabled bool) error {
	bucket = s.normalizeObjectBucket(bucket)
	return s.db.Update(func(tx *bolt.Tx) error {
		objects := tx.Bucket([]byte("objects"))
		latest := tx.Bucket([]byte("object_latest"))
		if !versioningEnabled {
			if objects.Get(objectKey(bucket, key)) == nil && latest.Get(latestIndexKey(bucket, key)) == nil {
				return ErrNotFound
			}
			return s.deleteAllVersionsTx(tx, bucket, key)
		}
		if versionID == "" {
			vid := latest.Get(latestIndexKey(bucket, key))
			if vid == nil {
				if objects.Get(objectKey(bucket, key)) == nil {
					return ErrNotFound
				}
				return objects.Delete(objectKey(bucket, key))
			}
			versionID = string(vid)
		}
		vk := versionObjectKey(bucket, key, versionID)
		if objects.Get(vk) == nil {
			return ErrNotFound
		}
		if err := objects.Delete(vk); err != nil {
			return err
		}
		currentLatest := string(latest.Get(latestIndexKey(bucket, key)))
		if currentLatest != versionID {
			return nil
		}
		var nextID string
		var nextMod ObjectRecord
		prefix := []byte(bucket + "\x00" + key + "\x00")
		c := objects.Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec ObjectRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				continue
			}
			if nextID == "" || rec.LastModified.After(nextMod.LastModified) {
				nextID = rec.VersionID
				nextMod = rec
			}
		}
		if nextID == "" {
			return latest.Delete(latestIndexKey(bucket, key))
		}
		return latest.Put(latestIndexKey(bucket, key), []byte(nextID))
	})
}

// ListObjects returns the latest object per key.
func (s *Store) ListObjects(bucket, prefix string, maxKeys int) ([]ObjectRecord, error) {
	bucket = s.normalizeObjectBucket(bucket)
	var out []ObjectRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		latest := tx.Bucket([]byte("object_latest"))
		objects := tx.Bucket([]byte("objects"))
		bucketPrefix := []byte(bucket + "\x00")
		seen := map[string]bool{}

		// Versioned latest pointers.
		c := latest.Cursor()
		for k, vid := c.Seek(bucketPrefix); k != nil && strings.HasPrefix(string(k), string(bucketPrefix)); k, vid = c.Next() {
			parts := splitObjectKey(k)
			if parts[0] != bucket {
				break
			}
			objKey := parts[1]
			if prefix != "" && !strings.HasPrefix(objKey, prefix) {
				continue
			}
			data := objects.Get(versionObjectKey(bucket, objKey, string(vid)))
			if data == nil {
				continue
			}
			var rec ObjectRecord
			if err := json.Unmarshal(data, &rec); err != nil {
				return err
			}
			if rec.IsDeleteMarker {
				continue
			}
			out = append(out, rec)
			seen[objKey] = true
			if maxKeys > 0 && len(out) >= maxKeys {
				return nil
			}
		}

		// Legacy non-versioned flat keys.
		c = objects.Cursor()
		for k, v := c.Seek(bucketPrefix); k != nil; k, v = c.Next() {
			parts := splitObjectKey(k)
			if parts[0] != bucket {
				break
			}
			if strings.Count(string(k), "\x00") > 1 {
				continue
			}
			objKey := parts[1]
			if seen[objKey] {
				continue
			}
			if prefix != "" && !strings.HasPrefix(objKey, prefix) {
				if objKey < prefix {
					continue
				}
				break
			}
			var rec ObjectRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			if maxKeys > 0 && len(out) >= maxKeys {
				break
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, err
}

// ListObjectVersions returns all stored versions (latest first per key).
func (s *Store) ListObjectVersions(bucket, prefix string, maxKeys int) ([]ObjectRecord, error) {
	bucket = s.normalizeObjectBucket(bucket)
	var out []ObjectRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		objects := tx.Bucket([]byte("objects"))
		bucketPrefix := []byte(bucket + "\x00")
		c := objects.Cursor()
		for k, v := c.Seek(bucketPrefix); k != nil; k, v = c.Next() {
			b, objKey, versionID := splitVersionObjectKey(k)
			if b == "" {
				parts := splitObjectKey(k)
				if parts[0] != bucket {
					break
				}
				if strings.Count(string(k), "\x00") > 1 {
					continue
				}
				b, objKey, versionID = bucket, parts[1], ""
			} else if b != bucket {
				break
			}
			if prefix != "" && !strings.HasPrefix(objKey, prefix) {
				continue
			}
			var rec ObjectRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if versionID != "" && rec.VersionID == "" {
				rec.VersionID = versionID
			}
			out = append(out, rec)
			if maxKeys > 0 && len(out) >= maxKeys {
				break
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].LastModified.After(out[j].LastModified)
	})
	return out, err
}

// ListObjectVersionIDs returns all version IDs for a key (newest first).
func (s *Store) ListObjectVersionIDs(bucket, key string) ([]string, error) {
	bucket = s.normalizeObjectBucket(bucket)
	versions, err := s.ListObjectVersions(bucket, key, 0)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, v := range versions {
		if v.Key == key && v.VersionID != "" {
			ids = append(ids, v.VersionID)
		}
	}
	return ids, nil
}

// TotalObjectBytes sums latest object sizes only.
func (s *Store) totalLatestObjectBytes() (int64, error) {
	buckets, err := s.ListBuckets()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, b := range buckets {
		objs, err := s.ListObjects(b.Name, "", 0)
		if err != nil {
			continue
		}
		for _, o := range objs {
			total += o.Size
		}
	}
	return total, nil
}
