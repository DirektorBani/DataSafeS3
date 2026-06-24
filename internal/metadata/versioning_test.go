package metadata_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
)

func TestObjectVersioningMetadata(t *testing.T) {
	s := openStore(t)
	_ = s.PutBucket(metadata.BucketRecord{Name: "b", CreatedAt: time.Now().UTC(), Versioning: true})
	now := time.Now().UTC()
	v1 := metadata.ObjectRecord{Bucket: "b", Key: "file.txt", Size: 3, ETag: `"a"`, VersionID: "v1", LastModified: now}
	v2 := metadata.ObjectRecord{Bucket: "b", Key: "file.txt", Size: 5, ETag: `"b"`, VersionID: "v2", LastModified: now.Add(time.Second)}
	if err := s.PutObjectVersioned(v1); err != nil {
		t.Fatal(err)
	}
	if err := s.PutObjectVersioned(v2); err != nil {
		t.Fatal(err)
	}
	latest, err := s.GetObject("b", "file.txt")
	if err != nil || latest.VersionID != "v2" {
		t.Fatalf("latest %+v err %v", latest, err)
	}
	got, err := s.GetObjectVersion("b", "file.txt", "v1")
	if err != nil || got.Size != 3 {
		t.Fatalf("v1 %+v err %v", got, err)
	}
	versions, err := s.ListObjectVersions("b", "", 0)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions %v err %v", versions, err)
	}
	list, err := s.ListObjects("b", "", 0)
	if err != nil || len(list) != 1 || list[0].VersionID != "v2" {
		t.Fatalf("list %+v err %v", list, err)
	}
	if err := s.DeleteObjectVersion("b", "file.txt", "v2", true); err != nil {
		t.Fatal(err)
	}
	latest, err = s.GetObject("b", "file.txt")
	if err != nil || latest.VersionID != "v1" {
		t.Fatalf("after delete latest %+v err %v", latest, err)
	}
}

func TestObjectNonVersionedOverwrite(t *testing.T) {
	s := openStore(t)
	_ = s.PutBucket(metadata.BucketRecord{Name: "b", CreatedAt: time.Now().UTC()})
	rec := metadata.ObjectRecord{Bucket: "b", Key: "k", Size: 1, ETag: `"a"`, LastModified: time.Now().UTC()}
	if err := s.PutObject(rec); err != nil {
		t.Fatal(err)
	}
	rec.Size = 2
	rec.ETag = `"b"`
	if err := s.PutObject(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetObject("b", "k")
	if err != nil || got.Size != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestStorageObjectVersioning(t *testing.T) {
	dir := t.TempDir()
	b, err := storage.NewFSBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body1 := []byte("one")
	body2 := []byte("two-long")
	if _, err := b.PutObjectVersion(ctx, "test", "doc.txt", "aaa", bytes.NewReader(body1), int64(len(body1)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PutObjectVersion(ctx, "test", "doc.txt", "bbb", bytes.NewReader(body2), int64(len(body2)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, info, err := b.GetObjectVersion(ctx, "test", "doc.txt", "aaa")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body1) || info.Size != int64(len(body1)) {
		t.Fatalf("v1 got %q size %d", got, info.Size)
	}
	rc, info, err = b.GetObjectVersion(ctx, "test", "doc.txt", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body2) || info.Size != int64(len(body2)) {
		t.Fatalf("v2 got %q size %d", got, info.Size)
	}
}

func openStoreForVersioning(t *testing.T) *metadata.Store {
	t.Helper()
	s, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServiceVersioningIntegration(t *testing.T) {
	dir := t.TempDir()
	meta, err := metadata.OpenBolt(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = meta.Close() })
	backend, err := storage.NewFSBackend(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	_ = backend.CreateBucket("ver")
	_ = meta.PutBucket(metadata.BucketRecord{Name: "ver", CreatedAt: time.Now().UTC(), Versioning: true})

	// minimal inline test via metadata + storage only (service tested in s3 package)
	v1 := metadata.ObjectRecord{Bucket: "ver", Key: "x", Size: 4, VersionID: "id1", LastModified: time.Now().UTC()}
	v2 := metadata.ObjectRecord{Bucket: "ver", Key: "x", Size: 8, VersionID: "id2", LastModified: time.Now().UTC()}
	_ = meta.PutObjectVersioned(v1)
	_ = meta.PutObjectVersioned(v2)
	latest, _ := meta.GetObject("ver", "x")
	if latest.VersionID != "id2" {
		t.Fatalf("expected id2 got %s", latest.VersionID)
	}
	all, _ := meta.ListObjectVersions("ver", "", 0)
	if len(all) != 2 {
		t.Fatalf("expected 2 versions got %d", len(all))
	}
}
