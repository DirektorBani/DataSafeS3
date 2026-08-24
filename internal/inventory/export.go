package inventory

import (
	"fmt"
	"time"
)

// ObjectMeta is the listing subset needed for inventory (avoids importing metadata in callers' tests).
type ObjectMeta struct {
	Key            string
	Size           int64
	LastModified   time.Time
	StorageClass   string
	VersionID      string
	LegalHold      bool
	RetentionUntil *time.Time
}

// BucketLockMeta carries bucket-level Object Lock settings into each row.
type BucketLockMeta struct {
	ObjectLock    bool
	RetentionDays int
	RetentionMode string
}

// BuildRows maps object listing + bucket lock flags into inventory rows.
func BuildRows(bucket string, lock BucketLockMeta, objs []ObjectMeta) []Row {
	out := make([]Row, 0, len(objs))
	for _, o := range objs {
		row := Row{
			Bucket:              bucket,
			Key:                 o.Key,
			Size:                o.Size,
			LastModified:        o.LastModified,
			StorageClass:        o.StorageClass,
			VersionID:           o.VersionID,
			ObjectLockEnabled:   lock.ObjectLock,
			BucketRetentionDays: lock.RetentionDays,
			RetentionMode:       lock.RetentionMode,
			LegalHold:           o.LegalHold,
		}
		if o.RetentionUntil != nil {
			row.RetentionUntil = o.RetentionUntil.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	return out
}

// ValidateExportRequest checks required fields for a manual CSV job.
func ValidateExportRequest(bucket, format string) error {
	if bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		return fmt.Errorf("unsupported format %q (csv only in this release)", format)
	}
	return nil
}
