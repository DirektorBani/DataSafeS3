package s3_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestCreateBucketOwnedByCallerKeepsEncryption(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_ADDR", "")
	t.Setenv("STORAGE_VAULT_ADDR", "")
	t.Setenv("STORAGE_SSE_MASTER_KEY", "test-sse-master-key")
	dir := t.TempDir()
	creds := auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}
	srv, err := api.NewServer(api.Config{
		DataDir:       dir,
		Region:        "us-east-1",
		AccessKey:     creds.AccessKey,
		SecretKey:     creds.SecretKey,
		AdminUser:     "admin",
		AdminPassword: "admin",
		JWTSecret:     "test",
		SSEMasterKey:  "test-sse-master-key",
		Metadata:      metadata.Config{Backend: metadata.BackendBolt, DataDir: dir},
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
	if err := srv.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}

	const bucket = "kms2"
	if err := srv.Svc().CreateBucket(t.Context(), bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	_, rec, err := srv.Svc().ResolveBucketKey("admin", bucket)
	if err != nil {
		t.Fatal(err)
	}
	rec, err = srv.Svc().SetBucketEncryption(rec, kms.SSEAlgorithmKMS, "")
	if err != nil {
		t.Fatal(err)
	}
	rec.Versioning = true
	if err := srv.Meta().UpdateBucket(rec); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	status, body := signedPut(t, ts.URL+"/"+bucket, creds)
	if status != http.StatusConflict || !strings.Contains(body, "BucketAlreadyOwnedByYou") {
		t.Fatalf("create existing: %d %s", status, body)
	}
	got := getRaw(t, ts.URL, creds, "/"+bucket+"?encryption")
	if !strings.Contains(got, "aws:kms") {
		t.Fatalf("encryption cleared: %s", got)
	}
	again, err := srv.Meta().GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Versioning || again.DefaultEncryption != kms.SSEAlgorithmKMS {
		t.Fatalf("config changed: versioning %v encryption %q", again.Versioning, again.DefaultEncryption)
	}
	n := 0
	all, err := srv.Meta().ListBuckets()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range all {
		if b.Name == bucket {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("buckets named %s: %d", bucket, n)
	}

	status, body = signedPut(t, ts.URL+"/fresh-bucket", creds)
	if status != http.StatusOK {
		t.Fatalf("create new: %d %s", status, body)
	}
}

func signedPut(t *testing.T, rawURL string, creds auth.Credentials) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
