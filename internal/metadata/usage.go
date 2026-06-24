package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

type UsageCounters struct {
	UploadBytes   int64 `json:"upload_bytes"`
	DownloadBytes int64 `json:"download_bytes"`
}

type UsageSnapshot struct {
	Date         string `json:"date"`
	StorageBytes int64  `json:"storage_bytes"`
	ObjectCount  int    `json:"object_count"`
	BucketCount  int    `json:"bucket_count"`
}

type BucketUsage struct {
	Name         string     `json:"name"`
	ObjectCount  int        `json:"object_count"`
	TotalSize    int64      `json:"total_size"`
	LastActivity *time.Time `json:"last_activity,omitempty"`
	Owner        string     `json:"owner"`
}

func (s *Store) AddUsageBytes(upload, download int64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("usage_counters"))
		var c UsageCounters
		if data := b.Get([]byte("global")); data != nil {
			_ = json.Unmarshal(data, &c)
		}
		c.UploadBytes += upload
		c.DownloadBytes += download
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return b.Put([]byte("global"), data)
	})
}

func (s *Store) GetUsageCounters() (UsageCounters, error) {
	var c UsageCounters
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("usage_counters")).Get([]byte("global"))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &c)
	})
	return c, err
}

func (s *Store) PutUsageSnapshot(snap UsageSnapshot) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("usage_snapshots")).Put([]byte(snap.Date), data)
	})
}

func (s *Store) ListUsageSnapshots(days int) ([]UsageSnapshot, error) {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	var out []UsageSnapshot
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("usage_snapshots")).ForEach(func(k, v []byte) error {
			date := string(k)
			if date < cutoff {
				return nil
			}
			var snap UsageSnapshot
			if err := json.Unmarshal(v, &snap); err != nil {
				return err
			}
			out = append(out, snap)
			return nil
		})
	})
	return out, err
}

func (s *Store) BucketUsageStats(filter BucketListFilter) ([]BucketUsage, error) {
	buckets, err := s.ListBucketsFiltered(filter)
	if err != nil {
		return nil, err
	}
	var out []BucketUsage
	for _, b := range buckets {
		objs, err := s.ListObjects(b.EffectiveStorageKey(), "", 0)
		if err != nil {
			continue
		}
		var total int64
		var last *time.Time
		for _, o := range objs {
			total += o.Size
			if last == nil || o.LastModified.After(*last) {
				t := o.LastModified
				last = &t
			}
		}
		out = append(out, BucketUsage{
			Name:         b.Name,
			ObjectCount:  len(objs),
			TotalSize:    total,
			LastActivity: last,
			Owner:        b.Owner,
		})
	}
	return out, nil
}

func (s *Store) CountObjects() (int, error) {
	buckets, err := s.ListBuckets()
	if err != nil {
		return 0, err
	}
	var n int
	for _, b := range buckets {
		objs, err := s.ListObjects(b.EffectiveStorageKey(), "", 0)
		if err != nil {
			continue
		}
		n += len(objs)
	}
	return n, nil
}

// OwnerUsage aggregates object count and bytes for buckets owned by a user.
func (s *Store) OwnerUsage(owner string) (objectCount int, totalBytes int64, err error) {
	stats, err := s.BucketUsageStats(BucketListFilter{Username: owner})
	if err != nil {
		return 0, 0, err
	}
	for _, st := range stats {
		objectCount += st.ObjectCount
		totalBytes += st.TotalSize
	}
	return objectCount, totalBytes, nil
}

func (s *Store) BucketObjectCount(bucket string) (int, error) {
	objs, err := s.ListObjects(bucket, "", 0)
	if err != nil {
		return 0, err
	}
	return len(objs), nil
}

func (s *Store) BucketTotalSize(bucket string) (int64, error) {
	objs, err := s.ListObjects(bucket, "", 0)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, o := range objs {
		total += o.Size
	}
	return total, nil
}

func (s *Store) UpdateBucket(rec BucketRecord) error {
	key := rec.EffectiveStorageKey()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("buckets"))
		if b.Get([]byte(key)) == nil {
			return ErrNotFound
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	})
}
