package inventory

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestInventoryJobFields(t *testing.T) {
	j := InventoryJob{
		ID:         "job-1",
		Bucket:     "reports",
		Prefix:     "2026/",
		Format:     "csv",
		Schedule:   "manual",
		DestBucket: "inventory",
		DestKey:    "reports/2026.csv",
		Status:     StatusPending,
	}
	if j.ID == "" || j.Bucket == "" || j.DestBucket == "" {
		t.Fatal("required fields empty")
	}
	if j.Format != "csv" && j.Format != "parquet" {
		t.Fatalf("unexpected format %q", j.Format)
	}
}

func TestInventoryJobZeroValue(t *testing.T) {
	var j InventoryJob
	if j.Schedule != "" {
		t.Fatal("zero value should have empty schedule")
	}
}

func TestWriteCSVIncludesLockColumns(t *testing.T) {
	until := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	rows := []Row{{
		Bucket:              "bk",
		Key:                 "a.txt",
		Size:                12,
		LastModified:        until,
		StorageClass:        "STANDARD",
		VersionID:           "v1",
		ObjectLockEnabled:   true,
		BucketRetentionDays: 30,
		RetentionMode:       "COMPLIANCE",
		RetentionUntil:      until.Format(time.RFC3339),
		LegalHold:           false,
	}}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "object_lock_enabled") || !strings.Contains(out, "retention_until") {
		t.Fatalf("missing lock columns: %s", out)
	}
	if !strings.Contains(out, "a.txt") || !strings.Contains(out, "COMPLIANCE") {
		t.Fatalf("missing row data: %s", out)
	}
}

func TestBuildRowsMapsRetention(t *testing.T) {
	until := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	rows := BuildRows("b1", BucketLockMeta{ObjectLock: true, RetentionDays: 7, RetentionMode: "GOVERNANCE"}, []ObjectMeta{{
		Key: "k", Size: 1, LastModified: until, RetentionUntil: &until, LegalHold: true,
	}})
	if len(rows) != 1 {
		t.Fatalf("len=%d", len(rows))
	}
	if !rows[0].ObjectLockEnabled || rows[0].RetentionUntil == "" || !rows[0].LegalHold {
		t.Fatalf("%+v", rows[0])
	}
}

func TestJobStoreRoundTrip(t *testing.T) {
	st := NewJobStore()
	j := st.Create("b", "p/", "csv", "", "")
	if j.ID == "" || j.Status != StatusPending {
		t.Fatalf("%+v", j)
	}
	ok := st.Update(j.ID, func(rec *JobRecord) {
		rec.Job.Status = StatusCompleted
		rec.CSV = []byte("csv")
	})
	if !ok {
		t.Fatal("update failed")
	}
	got, ok := st.Get(j.ID)
	if !ok || got.Job.Status != StatusCompleted || string(got.CSV) != "csv" {
		t.Fatalf("%+v ok=%v", got, ok)
	}
}

func TestValidateExportRequest(t *testing.T) {
	if err := ValidateExportRequest("", "csv"); err == nil {
		t.Fatal("expected bucket required")
	}
	if err := ValidateExportRequest("b", "parquet"); err == nil {
		t.Fatal("expected format error")
	}
	if err := ValidateExportRequest("b", "csv"); err != nil {
		t.Fatal(err)
	}
}
