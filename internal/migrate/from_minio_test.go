package migrate_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/migrate"
)

func TestFromMinIOCopiesVersionsLockAndResumes(t *testing.T) {
	srcURL, srcCreds, src := startNode(t)
	dstURL, dstCreds, _ := startNode(t)
	bucket := "mig"
	seedSource(t, src, srcURL, srcCreds, bucket)

	opt := options(t, srcURL, srcCreds, dstURL, dstCreds, bucket)
	opt.PartSize = 8
	opt.VerifyFull = true
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil {
		t.Fatalf("run: %v\nverdicts %#v\nwarnings %v\nerrors %v", err, ev.Verdicts, ev.Warnings, ev.Errors)
	}
	if !ev.OK() {
		t.Fatalf("verdicts %#v warnings %v", ev.Verdicts, ev.Warnings)
	}
	for _, name := range []string{"object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample"} {
		if ev.Verdicts[name] != "PASS" {
			t.Fatalf("%s %s", name, ev.Verdicts[name])
		}
	}
	warn := strings.Join(ev.Warnings, "\n")
	if !strings.Contains(warn, "DropMe") {
		t.Fatalf("warnings %s", warn)
	}

	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/notes.txt"); got != "two" {
		t.Fatalf("latest notes %q", got)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/wide.bin"); got != "0123456789abcdefghij" {
		t.Fatalf("multipart body %q", got)
	}
	head := headHeaders(t, dstURL, dstCreds, "/"+bucket+"/locked.txt")
	if head.Get("x-amz-meta-color") != "blue" {
		t.Fatalf("user metadata %#v", head)
	}
	if head.Get("x-amz-meta-datasafe-source-version") == "" {
		t.Fatal("source version metadata missing")
	}
	if code := statusOf(t, dstURL, dstCreds, http.MethodDelete, "/"+bucket+"/locked.txt", ""); code != http.StatusForbidden {
		t.Fatalf("locked delete status %d", code)
	}
	policy := getBody(t, dstURL, dstCreds, "/"+bucket+"?policy")
	if strings.Contains(policy, "StringEquals") || strings.Contains(policy, "DropMe") {
		t.Fatalf("unsupported policy stored: %s", policy)
	}
	if !strings.Contains(policy, "aws:SourceIp") {
		t.Fatalf("supported policy missing: %s", policy)
	}
	retention := getBody(t, dstURL, dstCreds, "/"+bucket+"/locked.txt?retention")
	if !strings.Contains(retention, "RetainUntilDate") {
		t.Fatalf("retention %s", retention)
	}
	before := statusOf(t, dstURL, dstCreds, http.MethodGet, "/"+bucket+"?versions", "")
	_ = before
	versionsBefore := getBody(t, dstURL, dstCreds, "/"+bucket+"?versions")

	ev2, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev2.OK() {
		t.Fatalf("second run: %v %#v %v", err, ev2.Verdicts, ev2.Errors)
	}
	versionsAfter := getBody(t, dstURL, dstCreds, "/"+bucket+"?versions")
	if versionsAfter != versionsBefore {
		t.Fatalf("resume duplicated versions\nbefore %s\nafter %s", versionsBefore, versionsAfter)
	}
}

func TestFromMinIOInterruptThenResume(t *testing.T) {
	srcURL, srcCreds, _ := startNode(t)
	dstURL, dstCreds, _ := startNode(t)
	bucket := "resume"
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"?versioning", versioningXML, nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/a.txt", "one", nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/a.txt", "two", nil, http.StatusOK)

	ctx, cancel := context.WithCancel(context.Background())
	opt := options(t, srcURL, srcCreds, dstURL, dstCreds, bucket)
	opt.Concurrency = 1
	opt.AfterObject = func(n int) {
		if n >= 1 {
			cancel()
		}
	}
	ev, err := migrate.Run(ctx, opt)
	if !errors.Is(err, migrate.ErrInterrupted) {
		t.Fatalf("expected interrupt, got %v errors %v", err, ev.Errors)
	}
	opt.AfterObject = nil
	ev, err = migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("resume: %v %#v %v", err, ev.Verdicts, ev.Errors)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/a.txt"); got != "two" {
		t.Fatalf("latest %q", got)
	}
	body := getBody(t, dstURL, dstCreds, "/"+bucket+"?versions")
	if strings.Count(body, "<Version>") != 2 {
		t.Fatalf("version count\n%s", body)
	}
}

func TestFromMinIORetriesNotLeader(t *testing.T) {
	_, creds, _ := startNode(t)
	dst, dstCreds := newNode(t)
	var failed atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Count(strings.Trim(r.URL.Path, "/"), "/") >= 1 && r.URL.RawQuery == "" {
			if failed.CompareAndSwap(false, true) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"not cluster leader"}`))
				return
			}
		}
		dst.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	srcURL, srcCreds, _ := startNode(t)
	bucket := "leader"
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/file.txt", "payload", nil, http.StatusOK)

	opt := options(t, srcURL, srcCreds, ts.URL, dstCreds, bucket)
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("run: %v %#v %v", err, ev.Verdicts, ev.Errors)
	}
	if !failed.Load() {
		t.Fatal("503 was not injected")
	}
	if got := getBody(t, ts.URL, creds, "/"+bucket+"/file.txt"); got != "payload" {
		// The wrapper signs with dstCreds. Read through the same handler with dstCreds.
		got = getBody(t, ts.URL, dstCreds, "/"+bucket+"/file.txt")
		if got != "payload" {
			t.Fatalf("body %q", got)
		}
	}
}

func TestFromMinIOLive(t *testing.T) {
	ep := os.Getenv("MINIO_ENDPOINT")
	if ep == "" {
		t.Skip("MINIO_ENDPOINT not set")
	}
	srcCreds := auth.Credentials{
		AccessKey: envOr("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: envOr("MINIO_SECRET_KEY", "minioadmin"),
	}
	bucket := "live-mig"
	mustStatus(t, ep, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, ep, srcCreds, http.MethodPut, "/"+bucket+"/hello.txt", "from-minio", map[string]string{"Content-Type": "text/plain", "x-amz-meta-origin": "minio"}, http.StatusOK)

	dstURL, dstCreds, _ := startNode(t)
	opt := options(t, ep, srcCreds, dstURL, dstCreds, bucket)
	opt.VerifyFull = true
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("live: %v %#v warnings %v errors %v", err, ev.Verdicts, ev.Warnings, ev.Errors)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/hello.txt"); got != "from-minio" {
		t.Fatalf("body %q", got)
	}
}

func startNode(t *testing.T) (string, auth.Credentials, *api.Server) {
	t.Helper()
	srv, creds := newNode(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, creds, srv
}

func newNode(t *testing.T) (*api.Server, auth.Credentials) {
	t.Helper()
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_ADDR", "")
	t.Setenv("STORAGE_VAULT_ADDR", "")
	t.Setenv("STORAGE_SSE_MASTER_KEY", "")
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
		Metadata:      metadata.Config{Backend: metadata.BackendBolt, DataDir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, creds
}

func options(t *testing.T, src string, srcCreds auth.Credentials, dst string, dstCreds auth.Credentials, bucket string) migrate.Options {
	t.Helper()
	return migrate.Options{
		Source:      migrate.Endpoint{URL: src, AccessKey: srcCreds.AccessKey, SecretKey: srcCreds.SecretKey, Region: "us-east-1"},
		Targets:     []migrate.Endpoint{{URL: dst, AccessKey: dstCreds.AccessKey, SecretKey: dstCreds.SecretKey, Region: "us-east-1"}},
		Buckets:     []string{bucket},
		StateDir:    t.TempDir(),
		ReportDir:   t.TempDir(),
		Concurrency: 2,
		PartSize:    8 << 20,
		VerifyFull:  true,
	}
}

const versioningXML = `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`

func seedSource(t *testing.T, srv *api.Server, base string, creds auth.Credentials, bucket string) {
	t.Helper()
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"?versioning", versioningXML, nil, http.StatusOK)
	lock := `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"?object-lock", lock, nil, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/notes.txt", "one", map[string]string{"Content-Type": "text/plain"}, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/notes.txt", "two", map[string]string{"Content-Type": "text/plain"}, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/gone.txt", "temp", nil, http.StatusOK)
	mustStatus(t, base, creds, http.MethodDelete, "/"+bucket+"/gone.txt", "", nil, http.StatusNoContent)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/locked.txt", "secret", map[string]string{
		"Content-Type":     "text/plain",
		"x-amz-meta-color": "blue",
	}, http.StatusOK)
	until := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	ret := `<Retention><Mode>COMPLIANCE</Mode><RetainUntilDate>` + until + `</RetainUntilDate></Retention>`
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/locked.txt?retention", ret, nil, http.StatusOK)
	hold := `<LegalHold><Status>ON</Status></LegalHold>`
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/locked.txt?legal-hold", hold, nil, http.StatusOK)
	tags := `<Tagging><TagSet><Tag><Key>env</Key><Value>lab</Value></Tag></TagSet></Tagging>`
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/locked.txt?tagging", tags, nil, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/wide.bin", "0123456789abcdefghij", map[string]string{"Content-Type": "application/octet-stream"}, http.StatusOK)
	policy := `{
	  "Version":"2012-10-17",
	  "Statement":[
	    {"Sid":"Keep","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::mig/*","Condition":{"IpAddress":{"aws:SourceIp":"10.0.0.0/8"}}},
	    {"Sid":"DropMe","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::mig/*","Condition":{"StringEquals":{"s3:prefix":"x"}}}
	  ]
	}`
	rec, err := srv.Meta().GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	key := rec.StorageKey
	if key == "" {
		key = rec.Name
	}
	if err := srv.Meta().SetBucketPolicy(key, policy); err != nil {
		t.Fatal(err)
	}
	life := `<LifecycleConfiguration><Rule><ID>keep</ID><Prefix>logs/</Prefix><Status>Enabled</Status><Expiration><Days>30</Days></Expiration><Transition><Days>7</Days><StorageClass>GLACIER</StorageClass></Transition></Rule></LifecycleConfiguration>`
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"?lifecycle", life, nil, http.StatusOK)
}

func mustStatus(t *testing.T, base string, creds auth.Credentials, method, path, body string, hdr map[string]string, want int) {
	t.Helper()
	if got := statusOf(t, base, creds, method, path, body, hdr); got != want {
		t.Fatalf("%s %s: status %d, want %d", method, path, got, want)
	}
}

func statusOf(t *testing.T, base string, creds auth.Credentials, method, path, body string, hdr ...map[string]string) int {
	t.Helper()
	resp := do(t, base, creds, method, path, body, hdr...)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, b)
	}
	return resp.StatusCode
}

func getBody(t *testing.T, base string, creds auth.Credentials, path string) string {
	t.Helper()
	resp := do(t, base, creds, http.MethodGet, path, "")
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s %d %s", path, resp.StatusCode, b)
	}
	return string(b)
}

func headHeaders(t *testing.T, base string, creds auth.Credentials, path string) http.Header {
	t.Helper()
	resp := do(t, base, creds, http.MethodHead, path, "")
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("HEAD %s %d %s", path, resp.StatusCode, b)
	}
	return resp.Header
}

func do(t *testing.T, base string, creds auth.Credentials, method, path, body string, hdr ...map[string]string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" || method == http.MethodPut || method == http.MethodPost {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hdr {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	if err := auth.SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
