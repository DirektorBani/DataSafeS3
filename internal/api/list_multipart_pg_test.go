package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresListMultipartUploadsActive(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	t.Setenv("STORAGE_HA_ENABLED", "false")
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_mpu_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})
	fresh, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Path = "/" + name

	dir := t.TempDir()
	srv, err := NewServer(Config{
		DataDir: dir, Region: "us-east-1",
		AccessKey: "testkey", SecretKey: "testsecret",
		AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
		Metadata: metadata.Config{
			Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(), DataDir: dir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	cfg, err := srv.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := srv.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}

	const logical = "photos"
	const storageKey = "o__user1__photos"
	if err := srv.Meta().PutBucket(metadata.BucketRecord{
		Name: logical, StorageKey: storageKey, Owner: "alice", OwnerID: "user1",
		CreatedAt: time.Now().UTC(), TenantID: metadata.DefaultTenantID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.Svc().Backend.CreateBucket(storageKey); err != nil {
		t.Fatal(err)
	}
	uploadID, err := srv.Svc().CreateMultipartUpload(ctx, storageKey, "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := srv.Meta().ListMultipart(storageKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].UploadID != uploadID {
		t.Fatalf("postgres list by storage key: %+v", listed)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	creds := auth.Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/"+logical+"?uploads", nil)
	req.Host = req.URL.Host
	if err := auth.SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %d: %s", resp.StatusCode, body)
	}
	text := string(body)
	if strings.Count(text, "<Upload>") != 1 || !strings.Contains(text, uploadID) {
		t.Fatalf("expected the active upload, got %s", text)
	}
}
