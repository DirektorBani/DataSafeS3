package inventory

import "time"

// Job status values.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// DefaultMaxObjects caps a single inventory scan (operator evidence, not full-warehouse dump).
const DefaultMaxObjects = 100_000

// InventoryJob describes a storage listing export job.
type InventoryJob struct {
	ID          string     `json:"id"`
	Bucket      string     `json:"bucket"`
	Prefix      string     `json:"prefix,omitempty"`
	Format      string     `json:"format"`   // csv
	Schedule    string     `json:"schedule"` // manual
	DestBucket  string     `json:"dest_bucket,omitempty"`
	DestKey     string     `json:"dest_key,omitempty"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	ObjectCount int        `json:"object_count,omitempty"`
	Truncated   bool       `json:"truncated,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// Row is one CSV/JSON inventory line with optional Object Lock evidence fields.
type Row struct {
	Bucket              string
	Key                 string
	Size                int64
	LastModified        time.Time
	StorageClass        string
	VersionID           string
	ObjectLockEnabled   bool
	BucketRetentionDays int
	RetentionMode       string
	RetentionUntil      string
	LegalHold           bool
}
