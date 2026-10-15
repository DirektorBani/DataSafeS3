package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// A killed leader used to commit DELETE FROM objects and never reach the
// INSERT. The row was gone, the file stayed on disk, and the resumed rewrap
// reported completed. Both statements now share one transaction, so an insert
// failure puts the previous row back.
func TestUnversionedPutKeepsRowWhenInsertFails(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS datasafe_test_fail_etag (etag text PRIMARY KEY);
		CREATE OR REPLACE FUNCTION datasafe_fail_etag() RETURNS trigger AS $$
		BEGIN
			IF EXISTS (SELECT 1 FROM datasafe_test_fail_etag WHERE etag = NEW.etag) THEN
				RAISE EXCEPTION 'injected insert failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS datasafe_fail_etag ON objects;
		CREATE TRIGGER datasafe_fail_etag BEFORE INSERT ON objects
		FOR EACH ROW EXECUTE FUNCTION datasafe_fail_etag()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS datasafe_fail_etag ON objects;
			DROP FUNCTION IF EXISTS datasafe_fail_etag();
			DROP TABLE IF EXISTS datasafe_test_fail_etag`)
	})

	bucket := "atomic-" + time.Now().Format("150405.000000")
	if err := s.PutBucket(metadata.BucketRecord{Name: bucket, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DeleteBucket(bucket) })
	if err := s.PutObject(metadata.ObjectRecord{
		Bucket: bucket, Key: "o29.txt", Size: 4, ETag: "keep",
		ContentType: "text/plain", LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	boom := "boom-" + bucket
	if _, err := s.pool.Exec(ctx, `INSERT INTO datasafe_test_fail_etag (etag) VALUES ($1)`, boom); err != nil {
		t.Fatal(err)
	}
	err = s.PutObject(metadata.ObjectRecord{
		Bucket: bucket, Key: "o29.txt", Size: 4, ETag: boom,
		ContentType: "text/plain", LastModified: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("insert was not refused")
	}
	rec, err := s.GetObject(bucket, "o29.txt")
	if err != nil {
		t.Fatalf("row deleted when the insert failed: %v", err)
	}
	if rec.ETag != "keep" {
		t.Fatalf("etag %q", rec.ETag)
	}
}
