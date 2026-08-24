package inventory

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

var csvHeader = []string{
	"bucket",
	"key",
	"size",
	"last_modified",
	"storage_class",
	"version_id",
	"object_lock_enabled",
	"bucket_retention_days",
	"retention_mode",
	"retention_until",
	"legal_hold",
}

// WriteCSV writes inventory rows as UTF-8 CSV (header + data).
func WriteCSV(w io.Writer, rows []Row) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			r.Bucket,
			r.Key,
			strconv.FormatInt(r.Size, 10),
			r.LastModified.UTC().Format(time.RFC3339),
			r.StorageClass,
			r.VersionID,
			strconv.FormatBool(r.ObjectLockEnabled),
			strconv.Itoa(r.BucketRetentionDays),
			r.RetentionMode,
			r.RetentionUntil,
			strconv.FormatBool(r.LegalHold),
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}
