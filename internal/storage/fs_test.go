package storage_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DirektorBani/datasafe/internal/storage"
)

func newTestBackend(t *testing.T) *storage.FSBackend {
	t.Helper()
	dir := t.TempDir()
	b, err := storage.NewFSBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CreateBucket("test"); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPutGetDeleteObject(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	body := []byte("hello world")
	etag, err := b.PutObject(ctx, "test", "a.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if etag == "" {
		t.Fatal("expected etag")
	}
	rc, info, err := b.GetObject(ctx, "test", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q want %q", got, body)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("size %d", info.Size)
	}
	if err := b.DeleteObject(ctx, "test", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.StatObject(ctx, "test", "a.txt"); err != storage.ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestMultipartUpload(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	uploadID := "upload-1"
	if err := b.CreateMultipartUpload(ctx, "test", "big.bin", uploadID); err != nil {
		t.Fatal(err)
	}
	p1 := []byte("part-one-")
	p2 := []byte("part-two")
	e1, err := b.UploadPart(ctx, "test", "big.bin", uploadID, 1, bytes.NewReader(p1), int64(len(p1)))
	if err != nil {
		t.Fatal(err)
	}
	e2, err := b.UploadPart(ctx, "test", "big.bin", uploadID, 2, bytes.NewReader(p2), int64(len(p2)))
	if err != nil {
		t.Fatal(err)
	}
	etag, err := b.CompleteMultipartUpload(ctx, "test", "big.bin", uploadID, []storage.PartInfo{
		{PartNumber: 1, ETag: e1},
		{PartNumber: 2, ETag: e2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if etag == "" {
		t.Fatal("expected etag")
	}
	rc, info, err := b.GetObject(ctx, "test", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want := append(p1, p2...)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if info.Size != int64(len(want)) {
		t.Fatalf("size %d", info.Size)
	}
}

func TestBucketLifecycle(t *testing.T) {
	dir := t.TempDir()
	b, err := storage.NewFSBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CreateBucket("b1"); err != nil {
		t.Fatal(err)
	}
	if err := b.CreateBucket("b1"); err != storage.ErrBucketExists {
		t.Fatalf("expected exists, got %v", err)
	}
	names, err := b.ListBuckets()
	if err != nil || len(names) != 1 {
		t.Fatalf("buckets %v err %v", names, err)
	}
	ctx := context.Background()
	_, err = b.PutObject(ctx, "b1", "x", bytes.NewReader([]byte("x")), 1, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteBucket("b1"); err != storage.ErrBucketNotEmpty {
		t.Fatalf("expected not empty, got %v", err)
	}
	_ = b.DeleteObject(ctx, "b1", "x")
	if err := b.DeleteBucket("b1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "buckets", "b1")); !os.IsNotExist(err) {
		t.Fatalf("bucket dir should be gone: %v", err)
	}
}
