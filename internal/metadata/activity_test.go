package metadata_test

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestActivityAppendList(t *testing.T) {
	s := openStore(t)
	_ = s.AppendActivity(metadata.ActivityRecord{
		User: "alice", Action: metadata.ActionLogin,
		ResourceType: "session", ResourceName: "alice",
		IPAddress: "127.0.0.1",
	})
	_ = s.AppendActivity(metadata.ActivityRecord{
		User: "bob", Action: metadata.ActionBucketCreated,
		ResourceType: "bucket", ResourceName: "data",
		IPAddress: "10.0.0.1",
	})
	res, err := s.ListActivity(metadata.ActivityFilter{Limit: 10})
	if err != nil || len(res.Events) != 2 {
		t.Fatalf("events %d err %v", len(res.Events), err)
	}
	res, err = s.ListActivity(metadata.ActivityFilter{User: "alice", Limit: 10})
	if err != nil || len(res.Events) != 1 {
		t.Fatalf("filtered %d err %v", len(res.Events), err)
	}
}

func TestUserCRUD(t *testing.T) {
	s := openStore(t)
	hash, _ := auth.HashPassword("secret")
	rec := metadata.UserRecord{
		ID: "u1", Username: "alice", Email: "a@test.com",
		PasswordHash: hash, Role: metadata.RoleUser,
		Status: metadata.StatusActive, CreatedAt: time.Now().UTC(),
	}
	if err := s.PutUser(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUserByUsername("alice")
	if err != nil || got.Username != "alice" {
		t.Fatalf("got %+v err %v", got, err)
	}
	got.Email = "new@test.com"
	if err := s.UpdateUser(got); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser("u1"); err != nil {
		t.Fatal(err)
	}
}

func TestUsageCounters(t *testing.T) {
	s := openStore(t)
	if err := s.AddUsageBytes(100, 50); err != nil {
		t.Fatal(err)
	}
	c, err := s.GetUsageCounters()
	if err != nil || c.UploadBytes != 100 || c.DownloadBytes != 50 {
		t.Fatalf("counters %+v err %v", c, err)
	}
}

func TestBucketUsageStats(t *testing.T) {
	s := openStore(t)
	_ = s.PutBucket(metadata.BucketRecord{Name: "b", CreatedAt: time.Now().UTC(), Owner: "alice"})
	_ = s.PutObject(metadata.ObjectRecord{
		Bucket: "b", Key: "k", Size: 42, LastModified: time.Now().UTC(),
	})
	stats, err := s.BucketUsageStats(metadata.BucketListFilter{Unfiltered: true})
	if err != nil || len(stats) != 1 || stats[0].TotalSize != 42 {
		t.Fatalf("stats %+v err %v", stats, err)
	}
}
