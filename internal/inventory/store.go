package inventory

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// JobRecord holds job metadata plus optional CSV payload for download.
type JobRecord struct {
	Job InventoryJob
	CSV []byte
}

// JobStore is an in-process registry for manual inventory jobs (Bolt and single-node Postgres).
// Not a durable queue — restart clears pending/completed jobs. Dest-bucket copies remain on disk.
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*JobRecord
}

// NewJobStore creates an empty job registry.
func NewJobStore() *JobStore {
	return &JobStore{jobs: make(map[string]*JobRecord)}
}

// Create registers a pending job and returns its id.
func (s *JobStore) Create(bucket, prefix, format, destBucket, destKey string) InventoryJob {
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		format = "csv"
	}
	id := newJobID()
	job := InventoryJob{
		ID:         id,
		Bucket:     bucket,
		Prefix:     prefix,
		Format:     format,
		Schedule:   "manual",
		DestBucket: destBucket,
		DestKey:    destKey,
		Status:     StatusPending,
		CreatedAt:  time.Now().UTC(),
	}
	s.mu.Lock()
	s.jobs[id] = &JobRecord{Job: job}
	s.mu.Unlock()
	return job
}

// Get returns a copy of the job record (CSV included when completed).
func (s *JobStore) Get(id string) (JobRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.jobs[id]
	if !ok {
		return JobRecord{}, false
	}
	cp := *rec
	if rec.CSV != nil {
		cp.CSV = append([]byte(nil), rec.CSV...)
	}
	return cp, true
}

// Update mutates job fields under lock.
func (s *JobStore) Update(id string, fn func(rec *JobRecord)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.jobs[id]
	if !ok {
		return false
	}
	fn(rec)
	return true
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "inv-" + hex.EncodeToString(b)
}
