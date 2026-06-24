package metadata_test

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestSearchShortBucketName(t *testing.T) {
	s := openStore(t)

	if err := s.PutBucket(metadata.BucketRecord{Name: "3", Owner: "admin", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBucket(metadata.BucketRecord{Name: "other", Owner: "admin", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	results, total, err := s.Search("3", "", false, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatalf("expected bucket 3 in search, got %d results", total)
	}
	found := false
	for _, r := range results {
		if r.Type == "bucket" && r.Name == "3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bucket 3 not found in %+v", results)
	}
}

func TestSearchExactAndPrefix(t *testing.T) {
	s := openStore(t)

	if err := s.PutBucket(metadata.BucketRecord{Name: "reports-2024", Owner: "admin", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutObject(metadata.ObjectRecord{
		Bucket: "reports-2024", Key: "report.txt", Size: 5,
		LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	results, total, err := s.Search("report", "", false, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("expected bucket and object hits for prefix report, got %d: %+v", total, results)
	}
}
