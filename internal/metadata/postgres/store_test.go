package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestPostgresConcurrentPutObjectUpsert(t *testing.T) {
	s := openTestPostgres(t)
	defer s.Close()
	bucket := fmt.Sprintf("pg-race-%d", time.Now().UnixNano())
	if err := s.PutBucket(metadata.BucketRecord{
		Name: bucket, Owner: "tester", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = s.DeleteObject(bucket, "same")
		_ = s.DeleteBucket(bucket)
	}()
	const n = 20
	var wg sync.WaitGroup
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.PutObject(metadata.ObjectRecord{
				Bucket: bucket, Key: "same",
				Size: int64(i + 1), ETag: fmt.Sprintf("etag-%d", i),
				ContentType: "text/plain", LastModified: time.Now().UTC(),
				CreatedAt: time.Now().UTC(),
			})
			if err != nil {
				errc <- err
			}
		}(i)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
	got, err := s.GetObject(bucket, "same")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("etag-%d", got.Size-1)
	if got.ETag != want {
		t.Fatalf("etag %q size %d are not one writer's row", got.ETag, got.Size)
	}
}

func TestMigrateConcurrentFreshDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
	})
	fresh, err := withDatabase(dsn, name)
	if err != nil {
		t.Fatal(err)
	}
	const n = 6
	var wg sync.WaitGroup
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(fresh, "")
			if err != nil {
				errc <- err
				return
			}
			s.Close()
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
}

func openTestPostgres(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	return s
}

func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
