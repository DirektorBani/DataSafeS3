package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

func testKEKEnv(t *testing.T, enabled bool) {
	t.Helper()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ENABLED", "false")
	if enabled {
		t.Setenv("STORAGE_FIELD_ENCRYPTION_ENABLED", "true")
	}
	t.Setenv("STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID", "kek-itest")
	t.Setenv("STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY", base64.StdEncoding.EncodeToString(seed))
}

func TestPostgresFieldEncryption_PutAccessKey_encryptedAtRest(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	testKEKEnv(t, true)

	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer s.Close()

	fe, err := fieldenc.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	s.SetFieldEncryption(fe)

	ak := "AKIAFE" + time.Now().Format("150405")
	secret := "field-enc-secret-value"
	rec := metadata.AccessKeyRecord{
		AccessKey: ak,
		SecretKey: secret,
		Label:     "fe-test",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.PutAccessKey(rec); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.DeleteAccessKey(ak) }()

	var stored string
	if err := s.pool.QueryRow(context.Background(), `SELECT secret_key FROM access_keys WHERE access_key=$1`, ak).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Fatalf("expected enc:v1 in DB, got %q", stored)
	}

	got, err := s.GetAccessKey(ak)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretKey != secret {
		t.Fatalf("decrypted secret %q want %q", got.SecretKey, secret)
	}
}

func TestPostgresFieldEncryption_disabled_plaintext(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	testKEKEnv(t, false)

	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer s.Close()

	fe, err := fieldenc.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	s.SetFieldEncryption(fe)

	ak := "AKIAPL" + time.Now().Format("150405")
	secret := "plaintext-secret"
	rec := metadata.AccessKeyRecord{
		AccessKey: ak,
		SecretKey: secret,
		Label:     "plain-test",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.PutAccessKey(rec); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.DeleteAccessKey(ak) }()

	var stored string
	if err := s.pool.QueryRow(context.Background(), `SELECT secret_key FROM access_keys WHERE access_key=$1`, ak).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != secret {
		t.Fatalf("expected plaintext in DB, got %q", stored)
	}
	got, err := s.GetAccessKey(ak)
	if err != nil || got.SecretKey != secret {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}
