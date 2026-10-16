package migrate_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
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
	for _, name := range []string{"object_count", "bytes", "delete_markers", "etag", "object_lock", "content_sample", "encryption"} {
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
	if !strings.Contains(retention, "RetainUntilDate") || !strings.Contains(retention, "COMPLIANCE") {
		t.Fatalf("retention %s", retention)
	}
	gov := `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>GOVERNANCE</Mode><Days>1</Days></DefaultRetention></Rule></ObjectLockConfiguration>`
	mustStatus(t, dstURL, dstCreds, http.MethodPut, "/"+bucket+"?object-lock", gov, nil, http.StatusOK)
	retention = getBody(t, dstURL, dstCreds, "/"+bucket+"/locked.txt?retention")
	if !strings.Contains(retention, "COMPLIANCE") {
		t.Fatalf("bucket mode overwrote object retention: %s", retention)
	}
	life := getBody(t, dstURL, dstCreds, "/"+bucket+"?lifecycle")
	if !strings.Contains(life, "logs/") {
		t.Fatalf("lifecycle prefix dropped: %s", life)
	}
	versions := getBody(t, dstURL, dstCreds, "/"+bucket+"?versions")
	mark := strings.Index(versions, "<DeleteMarker>")
	if mark < 0 || !strings.Contains(versions[mark:], "<Version>") {
		t.Fatalf("delete marker is not between versions\n%s", versions)
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

func TestFromMinIORefusesWidenedLifecycle(t *testing.T) {
	srcURL, srcCreds, src := startNode(t)
	dstURL, dstCreds, _ := startNode(t)
	bucket := "wide"
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/hello.txt", "hi", nil, http.StatusOK)
	bad := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>nc</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><NoncurrentVersionExpiration><NoncurrentDays>3</NoncurrentDays></NoncurrentVersionExpiration></Rule></LifecycleConfiguration>`
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"?lifecycle", bad, nil, http.StatusBadRequest)
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Has("lifecycle") {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(bad))
			return
		}
		src.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(wrapped.Close)
	opt := options(t, wrapped.URL, srcCreds, dstURL, dstCreds, bucket)
	ev, err := migrate.Run(context.Background(), opt)
	if err == nil || ev.OK() {
		t.Fatalf("expected lifecycle failure, got %v %#v", err, ev.Verdicts)
	}
	if !strings.Contains(strings.Join(ev.Errors, " "), "lifecycle") {
		t.Fatalf("errors %v", ev.Errors)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/hello.txt"); got != "hi" {
		t.Fatalf("object not copied: %q", got)
	}
	stored := getBody(t, dstURL, dstCreds, "/"+bucket+"?lifecycle")
	if strings.Contains(stored, "<Rule>") || strings.Contains(stored, "<Days>") {
		t.Fatalf("lifecycle was widened: %s", stored)
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

func TestResumeCopiesWhenJournalIsAheadOfTarget(t *testing.T) {
	srcURL, srcCreds, _ := startNode(t)
	dstURL, dstCreds, _ := startNode(t)
	bucket := "ahead"
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/blob.bin", "payload", nil, http.StatusOK)
	mustStatus(t, dstURL, dstCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)

	opt := options(t, srcURL, srcCreds, dstURL, dstCreds, bucket)
	st, err := migrate.OpenState(opt.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Mark(bucket, "blob.bin", "null", "copied", "deadbeef", 7); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("resume: %v %#v %v", err, ev.Verdicts, ev.Errors)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/blob.bin"); got != "payload" {
		t.Fatalf("journal skipped a missing object: %q", got)
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

func TestFromMinIOKeepsTargetKMSAndSHA(t *testing.T) {
	srcURL, srcCreds, _ := startNode(t)
	dstURL, dstCreds, _ := startEncryptedNode(t)
	bucket := "kmsb"
	mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	for i, body := range []string{"alpha", "bravo", "charlie"} {
		mustStatus(t, srcURL, srcCreds, http.MethodPut, "/"+bucket+"/f"+string(rune('0'+i))+".txt", body, nil, http.StatusOK)
	}
	enc := `<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`
	mustStatus(t, dstURL, dstCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, dstURL, dstCreds, http.MethodPut, "/"+bucket+"?encryption", enc, nil, http.StatusOK)

	opt := options(t, srcURL, srcCreds, dstURL, dstCreds, bucket)
	opt.VerifyFull = true
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("run: %v %#v warnings %v errors %v", err, ev.Verdicts, ev.Warnings, ev.Errors)
	}
	if ev.Verdicts["etag"] != "PASS" || ev.Verdicts["encryption"] != "PASS" {
		t.Fatalf("verdicts %#v", ev.Verdicts)
	}
	gotEnc := getBody(t, dstURL, dstCreds, "/"+bucket+"?encryption")
	if !strings.Contains(gotEnc, "aws:kms") {
		t.Fatalf("target encryption cleared: %s", gotEnc)
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/f0.txt"); got != "alpha" {
		t.Fatalf("body %q", got)
	}
	srcETag := strings.Trim(headHeaders(t, srcURL, srcCreds, "/"+bucket+"/f0.txt").Get("ETag"), `"`)
	dstHead := headHeaders(t, dstURL, dstCreds, "/"+bucket+"/f0.txt")
	dstETag := strings.Trim(dstHead.Get("ETag"), `"`)
	if srcETag == "" || srcETag == dstETag {
		t.Fatalf("expected a KMS etag distinct from the plaintext MD5 %q %q", srcETag, dstETag)
	}
	if dstHead.Get("x-amz-server-side-encryption") != "aws:kms" {
		t.Fatalf("object stored without SSE: %#v", dstHead)
	}
}

func startEncryptedNode(t *testing.T) (string, auth.Credentials, *api.Server) {
	t.Helper()
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
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, creds, srv
}

func TestVerifyDownloadsWhenMultipartETagDiffers(t *testing.T) {
	_, srcCreds, src := startNode(t)
	dstURL, dstCreds, dst := startNode(t)
	bucket := "mpv"
	body := "0123456789abcdef"
	srcWrap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Has("versions") {
			rec := httptest.NewRecorder()
			src.Handler().ServeHTTP(rec, r)
			out := regexp.MustCompile(`(?s)<ETag>.*?</ETag>`).ReplaceAllString(rec.Body.String(), `<ETag>"0123456789abcdef0123456789abcdef-2"</ETag>`)
			for k, vs := range rec.Header() {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write([]byte(out))
			return
		}
		src.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(srcWrap.Close)
	var targetGets atomic.Int32
	dstWrap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "big.bin") && !r.URL.Query().Has("versions") {
			targetGets.Add(1)
		}
		if r.Method == http.MethodHead {
			rec := httptest.NewRecorder()
			dst.Handler().ServeHTTP(rec, r)
			for k, vs := range rec.Header() {
				if strings.EqualFold(k, "x-amz-meta-datasafe-source-version") {
					continue
				}
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(rec.Code)
			return
		}
		dst.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(dstWrap.Close)

	mustStatus(t, srcWrap.URL, srcCreds, http.MethodPut, "/"+bucket, "", nil, http.StatusOK)
	mustStatus(t, srcWrap.URL, srcCreds, http.MethodPut, "/"+bucket+"/big.bin", body, nil, http.StatusOK)
	opt := options(t, srcWrap.URL, srcCreds, dstWrap.URL, dstCreds, bucket)
	opt.VerifyFull = true
	ev, err := migrate.Run(context.Background(), opt)
	if err != nil || !ev.OK() {
		t.Fatalf("run: %v %#v errors %v", err, ev.Verdicts, ev.Errors)
	}
	if ev.Verdicts["etag"] != "PASS" || ev.Verdicts["content_sample"] != "PASS" {
		t.Fatalf("verdicts %#v", ev.Verdicts)
	}
	if targetGets.Load() == 0 {
		t.Fatal("verifier failed the multipart etag without GET")
	}
	if got := getBody(t, dstURL, dstCreds, "/"+bucket+"/big.bin"); got != body {
		t.Fatalf("body %q", got)
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
	// MinIO rejects x-amz-* and Content-Type unless they are in the signature.
	// This seed is a plain object; the two-server test covers metadata.
	mustStatus(t, ep, srcCreds, http.MethodPut, "/"+bucket+"/hello.txt", "from-minio", nil, http.StatusOK)

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
	mustStatus(t, base, creds, http.MethodDelete, "/"+bucket+"/notes.txt", "", nil, http.StatusNoContent)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/notes.txt", "two", map[string]string{"Content-Type": "text/plain"}, http.StatusOK)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/gone.txt", "temp", nil, http.StatusOK)
	mustStatus(t, base, creds, http.MethodDelete, "/"+bucket+"/gone.txt", "", nil, http.StatusNoContent)
	mustStatus(t, base, creds, http.MethodPut, "/"+bucket+"/locked.txt", "secret", map[string]string{
		"Content-Type":     "text/plain",
		"x-amz-meta-color": "blue",
	}, http.StatusOK)
	until := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	ret := `<Retention xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Mode>COMPLIANCE</Mode><RetainUntilDate>` + until + `</RetainUntilDate></Retention>`
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
	life := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>keep</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`
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
