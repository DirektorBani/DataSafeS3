package postgres

import (
	"context"
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

func TestNullableFK_team_id(t *testing.T) {
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

	suffix := time.Now().Format("150405")
	bucket := "fk-null-team-" + suffix
	if err := s.PutBucket(metadata.BucketRecord{
		Name: bucket, Owner: "tester", TeamID: "", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("PutBucket with empty team_id: %v", err)
	}
	defer func() { _ = s.DeleteBucket(bucket) }()

	userID := "user-fk-" + suffix
	username := "userfk" + suffix
	if err := s.PutUser(metadata.UserRecord{
		ID: userID, Username: username, Email: username + "@test.local",
		PasswordHash: "hash", Role: metadata.RoleUser, Status: "active",
		TenantID: metadata.DefaultTenantID, TeamID: "", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("PutUser with empty team_id: %v", err)
	}
	defer func() { _ = s.DeleteUser(userID) }()

	ctx := context.Background()
	var bucketTeamID, userTeamID *string
	if err := s.pool.QueryRow(ctx, `SELECT team_id FROM buckets WHERE name=$1`, bucket).Scan(&bucketTeamID); err != nil {
		t.Fatalf("query bucket team_id: %v", err)
	}
	if bucketTeamID != nil {
		t.Fatalf("bucket team_id should be SQL NULL, got %q", *bucketTeamID)
	}
	if err := s.pool.QueryRow(ctx, `SELECT team_id FROM users WHERE id=$1`, userID).Scan(&userTeamID); err != nil {
		t.Fatalf("query user team_id: %v", err)
	}
	if userTeamID != nil {
		t.Fatalf("user team_id should be SQL NULL, got %q", *userTeamID)
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
