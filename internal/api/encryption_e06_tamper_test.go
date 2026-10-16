package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestE06LegacyClassifyBolt(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "")
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "legacy-key")
	runE06LegacyClassify(t, s, dir, true)
}

func TestE06LegacyClassifyBoltFlag(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "true")
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "legacy-key")
	runE06LegacyClassify(t, s, dir, false)
}

func TestE06LegacyClassifyPostgres(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "")
	s, dir := e06PostgresWithKey(t, "legacy-key")
	runE06LegacyClassify(t, s, dir, false)
}

func TestE06LegacyClassifyPostgresFlag(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "true")
	s, dir := e06PostgresWithKey(t, "legacy-key")
	runE06LegacyClassify(t, s, dir, false)
}

func TestE06ConsoleVaultKMSMixedCluster(t *testing.T) {
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "master-key")
	p, err := vaulttransit.Open(vaulttransit.Config{
		Provider: "vault-transit",
		Addr:     "http://127.0.0.1:1",
		RoleID:   "role",
		SecretID: "s3cret",
		Timeout:  time.Second,
	})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	s.SetVaultKMS(p)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "mixbox", nil)
	if err := s.Meta().UpsertNodeHeartbeat("old-node", "1.5.0", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mixbox/encryption", tok, []byte(`{"enabled":true,"provider":"vault-transit"}`), "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cluster_upgrade") {
		t.Fatalf("mixed console kms %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "vault_not_configured") {
		t.Fatalf("vault gate ran before the cluster gate: %s", rec.Body.String())
	}
}

func e06PostgresWithKey(t *testing.T, key string) (*api.Server, string) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "")
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_e06t_%d", time.Now().UnixNano())
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
	s, err := api.NewServer(api.Config{
		DataDir: dir, AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
		SSEMasterKey: key,
		Metadata: metadata.Config{
			Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(), DataDir: dir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func runE06LegacyClassify(t *testing.T, s *api.Server, dir string, background bool) {
	t.Helper()
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "tamper", nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	rnd := make([]byte, 64<<10)
	if _, err := rand.Read(rnd); err != nil {
		t.Fatal(err)
	}
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, rnd...)
	gzipBody := append([]byte{0x1f, 0x8b, 0x08, 0x00}, rnd...)
	zipBody := append([]byte{'P', 'K', 3, 4}, rnd...)
	short := rnd[:20]
	almost := rnd[:29]
	text := bytes.Repeat([]byte("plain-from-15\n"), 4000)
	zeros := make([]byte, 8192)
	bodies := map[string][]byte{
		"photo.jpg": jpeg,
		"real.gz":   gzipBody,
		"rand.zip":  zipBody,
		"rand.bin":  rnd,
		"short.bin": short,
		"b29.bin":   almost,
		"long.txt":  text,
		"zeros.bin": zeros,
	}
	for key, body := range bodies {
		plantUnmarkedLive(t, s, dir, "tamper", key, body)
	}
	if background {
		s.Svc().RunLegacyClassify(context.Background())
		for key := range bodies {
			assertPlaintextMarker(t, s, "tamper", key)
		}
	}
	for key, body := range bodies {
		got := s3Call(t, s, http.MethodGet, "/tamper/"+key, nil, nil)
		if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), body) {
			t.Fatalf("plaintext %s %d %d bytes", key, got.Code, got.Body.Len())
		}
		assertPlaintextMarker(t, s, "tamper", key)
	}

	plain := bytes.Repeat([]byte("legacy-sse-block-"), 180)
	sk := plantFormat0(t, s, dir, "tamper", "old.bin", plain)
	got := s3Call(t, s, http.MethodGet, "/tamper/old.bin", nil, nil)
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), plain) {
		t.Fatalf("legacy sse %d %d bytes", got.Code, got.Body.Len())
	}
	obj, err := s.Meta().GetObject(sk, "old.bin")
	if err != nil || obj.Encryption == nil || !obj.Encryption.Sealed() || obj.Encryption.SSEAlgorithm != "AES256" || obj.Encryption.FormatVersion != 0 {
		t.Fatalf("format0 marker %+v %v", obj.Encryption, err)
	}
	flipLastByte(t, dir, sk, "old.bin")
	code, body, closed := liveS3GET(t, ts.URL, "/tamper/old.bin")
	assertAuthFailed(t, code, body, closed)

	// A legacy blob that never authenticated is not positively encrypted.
	// Classification stores plaintext and the bytes stream.
	corrupt := plantFormat0(t, s, dir, "tamper", "corrupt.bin", plain)
	flipLastByte(t, dir, corrupt, "corrupt.bin")
	onDisk, err := os.ReadFile(filepath.Join(dir, "objects", "buckets", corrupt, "objects", "corrupt.bin"))
	if err != nil {
		t.Fatal(err)
	}
	got = s3Call(t, s, http.MethodGet, "/tamper/corrupt.bin", nil, nil)
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), onDisk) {
		t.Fatalf("unclassified tamper %d %d bytes", got.Code, got.Body.Len())
	}
	assertPlaintextMarker(t, s, "tamper", "corrupt.bin")

	payload := []byte("format-one-payload-0123456789")
	put := s3Call(t, s, http.MethodPut, "/tamper/new.bin", payload, map[string]string{
		"x-amz-server-side-encryption": "AES256",
	})
	if put.Code != http.StatusOK {
		t.Fatalf("format1 put %d %s", put.Code, put.Body.String())
	}
	got = s3Call(t, s, http.MethodGet, "/tamper/new.bin", nil, nil)
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), payload) {
		t.Fatalf("format1 before tamper %d %q", got.Code, got.Body.String())
	}
	flipLastByte(t, dir, sk, "new.bin")
	code, body, closed = liveS3GET(t, ts.URL, "/tamper/new.bin")
	assertAuthFailed(t, code, body, closed)
}

func assertPlaintextMarker(t *testing.T, s *api.Server, bucket, key string) {
	t.Helper()
	sk := storageKeyOf(t, s, bucket)
	obj, err := s.Meta().GetObject(sk, key)
	if err != nil || obj.Encryption == nil || !obj.Encryption.Plaintext || obj.Encryption.Sealed() {
		t.Fatalf("plaintext marker %s %+v %v", key, obj.Encryption, err)
	}
}

func plantFormat0(t *testing.T, s *api.Server, dir, bucket, key string, plain []byte) string {
	t.Helper()
	local, err := kms.NewLocal("legacy-key")
	if err != nil || local == nil {
		t.Fatal(err)
	}
	ct, err := local.EncryptFormat0(plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) < 28 {
		t.Fatalf("format0 too short: %d", len(ct))
	}
	return plantUnmarkedLive(t, s, dir, bucket, key, ct)
}

func plantUnmarkedLive(t *testing.T, s *api.Server, dir, bucket, key string, body []byte) string {
	t.Helper()
	sk := storageKeyOf(t, s, bucket)
	path := filepath.Join(dir, "objects", "buckets", sk, "objects", key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Meta().PutObject(metadata.ObjectRecord{
		Bucket: sk, Key: key, Size: int64(len(body)), ETag: "\"plant\"",
		ContentType: "application/octet-stream", LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return sk
}

func flipLastByte(t *testing.T, dir, storageKey, key string) {
	t.Helper()
	path := filepath.Join(dir, "objects", "buckets", storageKey, "objects", key)
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		t.Fatalf("flip %s: %v len %d", path, err, len(b))
	}
	b[len(b)-1] ^= 0xff
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func liveS3GET(t *testing.T, base, objectPath string) (int, []byte, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+objectPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SignRequest(req, rootCreds(), "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s did not finish within 2s: %v", objectPath, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", objectPath, err)
	}
	return resp.StatusCode, body, resp.Close
}

func assertAuthFailed(t *testing.T, code int, body []byte, closed bool) {
	t.Helper()
	if code != http.StatusInternalServerError || !strings.Contains(string(body), "message authentication failed") {
		t.Fatalf("tampered get %d %q", code, body)
	}
	if !closed {
		t.Fatal("connection was kept open after the authentication failure")
	}
}
