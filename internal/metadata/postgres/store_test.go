package postgres

import (
	"os"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestPostgresStoreIntegration(t *testing.T) {
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

	bucket := "pg-test-" + time.Now().Format("150405")
	if err := s.PutBucket(metadata.BucketRecord{
		Name: bucket, Owner: "tester", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.DeleteBucket(bucket) }()

	if err := s.PutObject(metadata.ObjectRecord{
		Bucket: bucket, Key: "a.txt", Size: 5, ETag: "abc",
		ContentType: "text/plain", LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := s.GetObject(bucket, "a.txt")
	if err != nil || obj.Key != "a.txt" {
		t.Fatalf("get object: %v %+v", err, obj)
	}
	results, total, err := s.Search("a.txt", "", false, 0, 10)
	if err != nil || total < 1 {
		t.Fatalf("search: total=%d err=%v results=%v", total, err, results)
	}
}

func TestMigrationVersionParse(t *testing.T) {
	if migrationVersion("migrations/001_init.up.sql") != 1 {
		t.Fatal("expected version 1")
	}
	if migrationVersion("migrations/010_foo.up.sql") != 10 {
		t.Fatal("expected version 10")
	}
}
