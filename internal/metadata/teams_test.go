package metadata

import (
	"testing"
	"time"
)

func TestBucketMatchesFilterTenant(t *testing.T) {
	filter := BucketListFilter{UserID: "u1", Username: "alice", TenantIDs: []string{"tenant-a"}}
	now := time.Now().UTC()
	if !BucketMatchesFilter(BucketRecord{Name: "t-bkt", TenantID: "tenant-a", CreatedAt: now}, filter) {
		t.Fatal("tenant bucket should match tenant filter")
	}
	if BucketMatchesFilter(BucketRecord{Name: "other", TenantID: "tenant-b", CreatedAt: now}, filter) {
		t.Fatal("foreign tenant bucket should not match")
	}
}

func TestBucketMatchesFilter(t *testing.T) {
	filter := BucketListFilter{UserID: "u1", Username: "alice", TeamIDs: []string{"t1"}}
	now := time.Now().UTC()

	cases := []struct {
		name string
		b    BucketRecord
		want bool
	}{
		{"own by owner_id", BucketRecord{Name: "a", OwnerID: "u1", CreatedAt: now}, true},
		{"own by username", BucketRecord{Name: "b", Owner: "alice", CreatedAt: now}, true},
		{"team bucket", BucketRecord{Name: "c", TeamID: "t1", CreatedAt: now}, true},
		{"admin legacy owner", BucketRecord{Name: "d", Owner: "admin", CreatedAt: now}, false},
		{"orphan bucket", BucketRecord{Name: "e", CreatedAt: now}, false},
		{"other user", BucketRecord{Name: "f", OwnerID: "u2", Owner: "bob", CreatedAt: now}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BucketMatchesFilter(tc.b, filter); got != tc.want {
				t.Fatalf("BucketMatchesFilter(%+v) = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}

func TestBucketUsageStatsFilteredByTenant(t *testing.T) {
	s, err := OpenBolt(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	for _, b := range []BucketRecord{
		{Name: "mine", Owner: "alice", OwnerID: "u1", CreatedAt: now},
		{Name: "tenant-bkt", TenantID: "tenant-a", Owner: "admin", CreatedAt: now},
	} {
		if err := s.PutBucket(b); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.PutObject(ObjectRecord{Bucket: "tenant-bkt", Key: "k", Size: 100, LastModified: now})

	filter := BucketListFilter{UserID: "u2", Username: "bob", TenantIDs: []string{"tenant-a"}}
	stats, err := s.BucketUsageStats(filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Name != "tenant-bkt" || stats[0].TotalSize != 100 {
		t.Fatalf("tenant member usage stats %+v err %v", stats, err)
	}
}

func TestListBucketsFilteredExcludesForeignBuckets(t *testing.T) {
	s, err := OpenBolt(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	for _, b := range []BucketRecord{
		{Name: "mine", Owner: "alice", OwnerID: "u1", CreatedAt: now},
		{Name: "admin-bucket", Owner: "admin", CreatedAt: now},
		{Name: "orphan", CreatedAt: now},
	} {
		if err := s.PutBucket(b); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListBucketsFiltered(BucketListFilter{UserID: "u1", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "mine" {
		t.Fatalf("expected only own bucket, got %+v", list)
	}
}
