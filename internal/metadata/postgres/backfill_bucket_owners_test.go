package postgres

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBackfillBucketOwnersSkipsDuplicateScope(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	suffix := time.Now().Format("150405.000")
	adminID := "admin-id-" + suffix
	adminUser := "admin" + suffix
	canonicalKey := "o__" + adminID + "__files"
	orphanKey := "o__" + adminUser + "__files"

	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password_hash, role, status, tenant_id, created_at)
		VALUES ($1, $2, $3, 'hash', 'admin', 'active', 'default', NOW())
		ON CONFLICT (id) DO NOTHING`, adminID, adminUser, adminUser+"@test.local")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() { _, _ = s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, adminID) }()

	_, err = s.pool.Exec(ctx, `
		INSERT INTO buckets (storage_key, name, owner, owner_id, tenant_id, created_at)
		VALUES ($1, 'files', $2, $3, 'default', NOW())`,
		canonicalKey, adminUser, adminID)
	if err != nil {
		t.Fatalf("insert canonical bucket: %v", err)
	}
	defer func() { _, _ = s.pool.Exec(ctx, `DELETE FROM buckets WHERE storage_key=$1`, canonicalKey) }()

	_, err = s.pool.Exec(ctx, `
		INSERT INTO buckets (storage_key, name, owner, owner_id, tenant_id, created_at)
		VALUES ($1, 'files', $2, NULL, 'default', NOW())`,
		orphanKey, adminUser)
	if err != nil {
		t.Fatalf("insert orphan bucket: %v", err)
	}
	defer func() { _, _ = s.pool.Exec(ctx, `DELETE FROM buckets WHERE storage_key=$1`, orphanKey) }()

	if err := s.backfillBucketOwners(ctx); err != nil {
		t.Fatal(err)
	}

	var canonicalOwnerID string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(owner_id,'') FROM buckets WHERE storage_key=$1`, canonicalKey).Scan(&canonicalOwnerID); err != nil {
		t.Fatalf("query canonical owner_id: %v", err)
	}
	if canonicalOwnerID != adminID {
		t.Fatalf("canonical bucket owner_id = %q, want %q", canonicalOwnerID, adminID)
	}
}

func TestBackfillBucketOwnersDeletesEmptyOrphan(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	suffix := time.Now().Format("150405.000")
	adminID := "admin-del-" + suffix
	adminUser := "admindel" + suffix
	canonicalKey := "o__" + adminID + "__files"
	orphanKey := "o__" + adminUser + "__files"

	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password_hash, role, status, tenant_id, created_at)
		VALUES ($1, $2, $3, 'hash', 'admin', 'active', 'default', NOW())
		ON CONFLICT (id) DO NOTHING`, adminID, adminUser, adminUser+"@test.local")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() { _, _ = s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, adminID) }()

	_, err = s.pool.Exec(ctx, `
		INSERT INTO buckets (storage_key, name, owner, owner_id, tenant_id, created_at)
		VALUES ($1, 'files', $2, $3, 'default', NOW())`,
		canonicalKey, adminUser, adminID)
	if err != nil {
		t.Fatalf("insert canonical bucket: %v", err)
	}
	defer func() { _, _ = s.pool.Exec(ctx, `DELETE FROM buckets WHERE storage_key=$1`, canonicalKey) }()

	_, err = s.pool.Exec(ctx, `
		INSERT INTO buckets (storage_key, name, owner, owner_id, tenant_id, created_at)
		VALUES ($1, 'files', $2, NULL, 'default', NOW())`,
		orphanKey, adminUser)
	if err != nil {
		t.Fatalf("insert orphan bucket: %v", err)
	}

	if err := s.backfillBucketOwners(ctx); err != nil {
		t.Fatal(err)
	}

	var orphanCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM buckets WHERE storage_key=$1`, orphanKey).Scan(&orphanCount); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if orphanCount != 0 {
		t.Fatalf("empty orphan bucket should be deleted, still present")
	}
}
