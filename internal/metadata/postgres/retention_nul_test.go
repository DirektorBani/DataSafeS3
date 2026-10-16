package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestPostgresRetentionSurvivesNULAndKeepsLatest(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer s.Close()
	_ = s.EnsureDefaultTenant()

	bucket := "pglock" + time.Now().Format("150405000")
	if err := s.PutBucket(metadata.BucketRecord{
		Name: bucket, Owner: "tester", CreatedAt: time.Now().UTC(),
		Versioning: true, ObjectLock: true,
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.DeleteBucket(bucket) }()
	brec, err := s.GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	storageKey := brec.EffectiveStorageKey()

	now := time.Now().UTC()
	v1 := metadata.ObjectRecord{
		Bucket: bucket, Key: "locked.txt", Size: 4, ETag: `"a"`, VersionID: "v1",
		LastModified: now, CreatedAt: now,
		Metadata: map[string]string{"note": "a\x00b"},
	}
	if err := s.PutObjectVersioned(v1); err != nil {
		t.Fatalf("put with NUL metadata: %v", err)
	}
	marker := metadata.ObjectRecord{
		Bucket: bucket, Key: "locked.txt", VersionID: "vm", IsDeleteMarker: true,
		LastModified: now.Add(time.Second), CreatedAt: now.Add(time.Second),
	}
	if err := s.PutObjectVersioned(marker); err != nil {
		t.Fatal(err)
	}
	until := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.SetObjectRetention(bucket, "locked.txt", "v1", until, "COMPLIANCE"); err != nil {
		t.Fatalf("retention: %v", err)
	}
	if err := s.SetObjectLegalHold(bucket, "locked.txt", "v1", true); err != nil {
		t.Fatalf("legal hold: %v", err)
	}
	got, err := s.GetObjectVersion(bucket, "locked.txt", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RetentionMode != "COMPLIANCE" || !got.LegalHold || got.RetentionUntil == nil || !got.RetentionUntil.Equal(until) {
		t.Fatalf("lock state %+v", got)
	}
	if got.Metadata["note"] != "ab" {
		t.Fatalf("metadata %+v", got.Metadata)
	}
	if _, ok := got.Metadata["\x01retention_mode"]; ok {
		t.Fatalf("private retention key leaked: %+v", got.Metadata)
	}
	var raw string
	if err := s.pool.QueryRow(context.Background(),
		`SELECT metadata::text FROM objects WHERE bucket=$1 AND key=$2 AND version_id=$3`,
		storageKey, "locked.txt", "v1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, `\u0000`) || strings.ContainsRune(raw, 0) {
		t.Fatalf("jsonb still contains a NUL escape: %s", raw)
	}
	var markerLatest, olderLatest bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT is_latest FROM objects WHERE bucket=$1 AND key=$2 AND version_id='vm'`,
		storageKey, "locked.txt").Scan(&markerLatest); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(context.Background(),
		`SELECT is_latest FROM objects WHERE bucket=$1 AND key=$2 AND version_id='v1'`,
		storageKey, "locked.txt").Scan(&olderLatest); err != nil {
		t.Fatal(err)
	}
	if !markerLatest || olderLatest {
		t.Fatalf("is_latest marker=%v older=%v", markerLatest, olderLatest)
	}
}
