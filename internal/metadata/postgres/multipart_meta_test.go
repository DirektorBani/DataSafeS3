package postgres

import (
	"os"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestPostgresMultipartKeepsUserMetadata(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	id := "upl" + time.Now().Format("150405.000000")
	defer func() {
		_ = s.DeleteMultipart(id)
		s.Close()
	}()
	rec := metadata.MultipartRecord{
		UploadID:    id,
		Bucket:      "b",
		Key:         "big.bin",
		Initiated:   time.Now().UTC(),
		ContentType: "application/octet-stream",
		Metadata: map[string]string{
			"X-Amz-Meta-Datasafe-Source-Version": "src-version-1",
			"X-Amz-Meta-Color":                   "blue",
		},
	}
	if err := s.PutMultipart(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMultipart(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != rec.ContentType {
		t.Fatalf("content type %q", got.ContentType)
	}
	if got.Metadata["X-Amz-Meta-Datasafe-Source-Version"] != "src-version-1" || got.Metadata["X-Amz-Meta-Color"] != "blue" {
		t.Fatalf("metadata %+v", got.Metadata)
	}
}
