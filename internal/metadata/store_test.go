package metadata_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func openStore(t *testing.T) *metadata.Store {
	t.Helper()
	s, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRegisterNodeAddressRoundTrip(t *testing.T) {
	s := openStore(t)
	if err := s.RegisterNodeAddress("node-b", "http://storage-b:9000"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNodeHeartbeat("node-b", "1.6.0", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	beats, err := s.ListNodeHeartbeats(time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, b := range beats {
		if b.NodeID == "node-b" {
			got = b.Address
		}
	}
	if got != "http://storage-b:9000" {
		t.Fatalf("address %q", got)
	}
}

func TestBucketCRUD(t *testing.T) {
	s := openStore(t)
	rec := metadata.BucketRecord{Name: "b", CreatedAt: time.Now().UTC(), Owner: "admin"}
	if err := s.PutBucket(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBucket("b")
	if err != nil || got.Name != "b" {
		t.Fatalf("got %+v err %v", got, err)
	}
	list, err := s.ListBuckets()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v err %v", list, err)
	}
	if err := s.DeleteBucket("b"); err != nil {
		t.Fatal(err)
	}
}

func TestObjectCRUD(t *testing.T) {
	s := openStore(t)
	_ = s.PutBucket(metadata.BucketRecord{Name: "b", CreatedAt: time.Now().UTC()})
	rec := metadata.ObjectRecord{
		Bucket: "b", Key: "k", Size: 5, ETag: `"abc"`, LastModified: time.Now().UTC(),
	}
	if err := s.PutObject(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetObject("b", "k")
	if err != nil || got.Key != "k" {
		t.Fatalf("got %+v err %v", got, err)
	}
	objs, err := s.ListObjects("b", "", 10)
	if err != nil || len(objs) != 1 {
		t.Fatalf("objs %v err %v", objs, err)
	}
	if err := s.DeleteObject("b", "k"); err != nil {
		t.Fatal(err)
	}
}
