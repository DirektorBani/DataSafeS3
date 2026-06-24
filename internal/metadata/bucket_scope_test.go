package metadata_test

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestMakeStorageKeyOwnerScoped(t *testing.T) {
	scope := metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: "user-1"}
	key := metadata.MakeStorageKey(scope, "data")
	if key != "o__user-1__data" {
		t.Fatalf("key = %q", key)
	}
}

func TestMakeStorageKeyLegacyEmptyOwner(t *testing.T) {
	scope := metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: ""}
	key := metadata.MakeStorageKey(scope, "legacy")
	if key != "legacy" {
		t.Fatalf("key = %q", key)
	}
}

func TestScopedBucketUniqueness(t *testing.T) {
	s, err := metadata.OpenBolt(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "shared", Owner: "alice", OwnerID: "u1", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "shared", Owner: "bob", OwnerID: "u2", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "shared", Owner: "bob", OwnerID: "u2", CreatedAt: now,
	}); err != metadata.ErrBucketExists {
		t.Fatalf("expected duplicate in same scope, got %v", err)
	}
}

func TestTenantScopedBucketUniqueness(t *testing.T) {
	s, err := metadata.OpenBolt(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "corp-data", Owner: "a", OwnerID: "u1", TenantID: "tenant-a", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "corp-data", Owner: "b", OwnerID: "u2", TenantID: "tenant-b", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	rec, err := s.ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: "tenant-a"}, "corp-data")
	if err != nil || rec.TenantID != "tenant-a" {
		t.Fatalf("resolve tenant-a: %+v err %v", rec, err)
	}
}

func TestResolveBucketTenantScopeIgnoresLegacyDefaultBucket(t *testing.T) {
	s, err := metadata.OpenBolt(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	if err := s.PutBucket(metadata.BucketRecord{
		Name: "shared", Owner: "admin", OwnerID: "admin-id", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: "tenant-x"}, "shared")
	if err != metadata.ErrNotFound {
		t.Fatalf("tenant scope should not fall back to legacy admin bucket, got %+v err %v", err, err)
	}
}
