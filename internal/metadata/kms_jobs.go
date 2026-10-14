package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

// KMSJob is one leader-owned encryption job. The cursor is the last object
// the leader finished. A new leader continues from that cursor.
type KMSJob struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	BucketKey     string     `json:"bucket_key"`
	Bucket        string     `json:"bucket"`
	KeyID         string     `json:"key_id"`
	TargetVersion int        `json:"target_version"`
	Cursor        string     `json:"cursor"`
	Status        string     `json:"status"`
	Total         int        `json:"total"`
	Done          int        `json:"done"`
	Errors        int        `json:"errors"`
	Samples       []string   `json:"samples,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	HolderID      string     `json:"holder_id,omitempty"`
}

func (s *Store) PutKMSJob(rec KMSJob) error {
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.kmsBucket(tx, "kms_jobs")
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

func (s *Store) GetKMSJob(id string) (KMSJob, error) {
	var rec KMSJob
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("kms_jobs"))
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

func (s *Store) ListKMSJobs() ([]KMSJob, error) {
	var out []KMSJob
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("kms_jobs"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec KMSJob
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}
