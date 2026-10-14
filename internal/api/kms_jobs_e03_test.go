package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newE03Server(t *testing.T, pgDSN string) *Server {
	t.Helper()
	t.Setenv("STORAGE_HA_ENABLED", "false")
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "")
	dir := t.TempDir()
	cfg := Config{
		DataDir: dir, Region: "us-east-1",
		AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
		SSEMasterKey: "test-sse-master-key",
		Metadata:     metadata.Config{Backend: metadata.BackendBolt, DataDir: dir},
	}
	if pgDSN != "" {
		cfg.Metadata = metadata.Config{Backend: metadata.BackendPostgres, PostgresDSN: pgDSN, DataDir: dir}
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Close()
		observability.ResetKMSJobMetricsForTest()
	})
	sys, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	sys.InitialSetupCompleted = true
	sys.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(sys); err != nil {
		t.Fatal(err)
	}
	return s
}

func kjobToken(t *testing.T, s *Server) string {
	t.Helper()
	rec := kjobDo(t, s, http.MethodPost, "/api/v1/admin/login", "", []byte(`{"username":"admin","password":"admin"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == "" {
		t.Fatal("missing token")
	}
	return body["token"]
}

func kjobDo(t *testing.T, s *Server, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func kjobBucket(t *testing.T, s *Server, tok, name string) {
	t.Helper()
	rec := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/"+name, tok, []byte(`{}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s %d %s", name, rec.Code, rec.Body.String())
	}
}

func kjobEnable(t *testing.T, s *Server, tok, name string) {
	t.Helper()
	rec := kjobDo(t, s, http.MethodPut, "/api/v1/buckets/"+name+"/encryption", tok, []byte(`{"enabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable %s %d %s", name, rec.Code, rec.Body.String())
	}
}

func kjobPut(t *testing.T, s *Server, bucket, key, body string) {
	t.Helper()
	_, err := s.Svc().PutObject(context.Background(), bucket, key, strings.NewReader(body), int64(len(body)), "text/plain", nil)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
}

func kjobRead(t *testing.T, s *Server, bucket, key string) string {
	t.Helper()
	rc, _, err := s.Svc().GetObject(context.Background(), bucket, key, "")
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func kjobStorageKey(t *testing.T, s *Server, name string) string {
	t.Helper()
	rec, err := s.Meta().GetBucket(name)
	if err != nil {
		t.Fatal(err)
	}
	return rec.EffectiveStorageKey()
}

func kjobObject(t *testing.T, s *Server, bucket, key string) metadata.ObjectRecord {
	t.Helper()
	rec, err := s.Meta().GetObject(kjobStorageKey(t, s, bucket), key)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func kjobDrain(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		s.stepKMSJobs(context.Background())
		jobs, err := s.Meta().ListKMSJobs()
		if err != nil {
			t.Fatal(err)
		}
		busy := false
		for _, job := range jobs {
			if job.Status == kms.JobPending || job.Status == kms.JobRunning {
				busy = true
			}
		}
		if !busy {
			return
		}
	}
	t.Fatal("kms jobs did not finish")
}

func kjobActivityHas(t *testing.T, s *Server, action, needle string) {
	t.Helper()
	list, err := s.Meta().ListActivity(metadata.ActivityFilter{Period: "all", Limit: 300})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range list.Events {
		if ev.Action == action && strings.Contains(ev.ResourceName, needle) {
			return
		}
	}
	t.Fatalf("activity %s %q missing in %d rows", action, needle, len(list.Events))
}

func TestE03RewrapRotateAndDestroy(t *testing.T) {
	s := newE03Server(t, "")
	runE03Jobs(t, s)
}

func TestE03RewrapRotateAndDestroyPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_kjob_%d", time.Now().UnixNano())
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
	s := newE03Server(t, fresh.String())
	runE03Jobs(t, s)
}

func runE03Jobs(t *testing.T, s *Server) {
	t.Helper()
	tok := kjobToken(t, s)
	kjobBucket(t, s, tok, "jobs")
	kjobPut(t, s, "jobs", "plain.txt", "plain-body")
	kjobEnable(t, s, tok, "jobs")
	kjobPut(t, s, "jobs", "old.txt", "old-body")

	rot := kjobDo(t, s, http.MethodPost, "/api/v1/kms/keys/"+kjobObject(t, s, "jobs", "old.txt").Encryption.KeyID+"/rotate", tok, nil)
	if rot.Code != http.StatusOK {
		t.Fatalf("rotate %d %s", rot.Code, rot.Body.String())
	}
	var rotated struct {
		KeyVersion int `json:"key_version"`
	}
	_ = json.Unmarshal(rot.Body.Bytes(), &rotated)
	if rotated.KeyVersion != 2 {
		t.Fatalf("version %d", rotated.KeyVersion)
	}
	kjobActivityHas(t, s, metadata.ActionKMSKeyRotated, "key_version=2")
	kjobPut(t, s, "jobs", "new.txt", "new-body")
	if kjobObject(t, s, "jobs", "new.txt").Encryption.KeyVersion != 2 {
		t.Fatal("new object should use the rotated version")
	}

	before := kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK
	start := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/jobs/encryption/rewrap", tok, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("rewrap %d %s", start.Code, start.Body.String())
	}
	var job metadata.KMSJob
	_ = json.Unmarshal(start.Body.Bytes(), &job)
	if job.ID == "" || job.Type != kms.JobEncryptExisting {
		t.Fatalf("job %#v", job)
	}
	view := kjobDo(t, s, http.MethodGet, "/api/v1/buckets/jobs/encryption", tok, nil)
	if !strings.Contains(view.Body.String(), job.ID) {
		t.Fatalf("active job missing %s", view.Body.String())
	}
	kjobDrain(t, s)
	done, err := s.Meta().GetKMSJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != kms.JobCompleted || done.Errors != 0 || done.Done != done.Total || done.Total < 3 {
		t.Fatalf("job result %+v", done)
	}
	if kjobRead(t, s, "jobs", "plain.txt") != "plain-body" || kjobRead(t, s, "jobs", "old.txt") != "old-body" || kjobRead(t, s, "jobs", "new.txt") != "new-body" {
		t.Fatal("object bytes changed")
	}
	if kjobObject(t, s, "jobs", "plain.txt").Encryption.KeyVersion != 2 || kjobObject(t, s, "jobs", "old.txt").Encryption.KeyVersion != 2 {
		t.Fatal("objects did not reach the target version")
	}
	if kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK == before {
		t.Fatal("rewrap did not replace the wrapped data key")
	}
	again := kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK
	second := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/jobs/encryption/rewrap", tok, nil)
	if second.Code != http.StatusAccepted {
		t.Fatalf("second rewrap %d %s", second.Code, second.Body.String())
	}
	kjobDrain(t, s)
	if kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK != again || kjobRead(t, s, "jobs", "old.txt") != "old-body" {
		t.Fatal("repeating rewrap changed an object already at the target version")
	}
	report := kjobDo(t, s, http.MethodGet, "/api/v1/kms/jobs/"+job.ID+"/report", tok, nil)
	if report.Code != http.StatusOK || !strings.Contains(report.Header().Get("Content-Disposition"), job.ID) {
		t.Fatalf("report %d %s", report.Code, report.Body.String())
	}

	kjobBucket(t, s, tok, "resume")
	kjobEnable(t, s, tok, "resume")
	kjobPut(t, s, "resume", "a.txt", "aaa")
	kjobPut(t, s, "resume", "b.txt", "bbb")
	keyID := kjobObject(t, s, "resume", "a.txt").Encryption.KeyID
	if rec := kjobDo(t, s, http.MethodPost, "/api/v1/kms/keys/"+keyID+"/rotate", tok, nil); rec.Code != http.StatusOK {
		t.Fatalf("resume rotate %d %s", rec.Code, rec.Body.String())
	}
	resume := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/resume/encryption/rewrap", tok, nil)
	if resume.Code != http.StatusAccepted {
		t.Fatalf("resume job %d %s", resume.Code, resume.Body.String())
	}
	var resumed metadata.KMSJob
	_ = json.Unmarshal(resume.Body.Bytes(), &resumed)
	a := kjobObject(t, s, "resume", "a.txt")
	resumed.Cursor = kms.Cursor(a.Key, a.VersionID)
	if err := s.Meta().PutKMSJob(resumed); err != nil {
		t.Fatal(err)
	}
	kjobDrain(t, s)
	if kjobObject(t, s, "resume", "a.txt").Encryption.KeyVersion != 1 {
		t.Fatal("cursor should leave the first object on the old version")
	}
	if kjobObject(t, s, "resume", "b.txt").Encryption.KeyVersion != 2 || kjobRead(t, s, "resume", "a.txt") != "aaa" || kjobRead(t, s, "resume", "b.txt") != "bbb" {
		t.Fatal("resume did not continue after the cursor")
	}
	c := kjobObject(t, s, "resume", "b.txt")
	c.Key = "c.txt"
	c.Encryption.KeyVersion = 1
	c.Encryption.WrappedDEK = "%%%"
	c.Encryption.FormatVersion = 1
	if err := s.Meta().PutObject(c); err != nil {
		t.Fatal(err)
	}
	bad := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/resume/encryption/rewrap", tok, nil)
	if bad.Code != http.StatusAccepted {
		t.Fatalf("bad job %d %s", bad.Code, bad.Body.String())
	}
	var badJob metadata.KMSJob
	_ = json.Unmarshal(bad.Body.Bytes(), &badJob)
	kjobDrain(t, s)
	finished, err := s.Meta().GetKMSJob(badJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Errors < 1 || !strings.Contains(strings.Join(finished.Samples, "\n"), "c.txt") {
		t.Fatalf("error sample missing %+v", finished)
	}

	share := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/jobs/shares", tok, []byte(`{"key":"old.txt","expires_in_sec":3600}`))
	if share.Code != http.StatusCreated {
		t.Fatalf("share %d %s", share.Code, share.Body.String())
	}
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(share.Body.Bytes(), &created)
	okDL := kjobDo(t, s, http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", "", nil)
	if okDL.Code != http.StatusOK || okDL.Body.String() != "old-body" {
		t.Fatalf("share download %d %s", okDL.Code, okDL.Body.String())
	}
	kjobActivityHas(t, s, metadata.ActionShareDownloaded, "key_id=")

	wrong := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/jobs/encryption/destroy", tok, []byte(`{"confirm":"other"}`))
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirm %d %s", wrong.Code, wrong.Body.String())
	}
	if kjobRead(t, s, "jobs", "old.txt") != "old-body" {
		t.Fatal("wrong confirm destroyed the key")
	}
	right := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/jobs/encryption/destroy", tok, []byte(`{"confirm":"jobs"}`))
	if right.Code != http.StatusAccepted {
		t.Fatalf("destroy %d %s", right.Code, right.Body.String())
	}
	kjobActivityHas(t, s, metadata.ActionKMSKeyDestroyed, kjobObject(t, s, "jobs", "old.txt").Encryption.KeyID)
	for _, key := range []string{"plain.txt", "old.txt", "new.txt"} {
		_, _, err := s.Svc().GetObject(context.Background(), "jobs", key, "")
		if err != s3.ErrKeyDestroyed {
			t.Fatalf("get %s after destroy: %v", key, err)
		}
	}
	dead := kjobDo(t, s, http.MethodGet, "/api/v1/buckets/jobs/objects/old.txt", tok, nil)
	if dead.Code != http.StatusForbidden || !strings.Contains(dead.Body.String(), s3.MsgKeyDestroyed) {
		t.Fatalf("console download %d %s", dead.Code, dead.Body.String())
	}
	denied := kjobDo(t, s, http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", "", nil)
	if denied.Code != http.StatusForbidden || !strings.Contains(denied.Body.String(), s3.MsgKeyDestroyed) {
		t.Fatalf("share after shred %d %s", denied.Code, denied.Body.String())
	}
	kjobActivityHas(t, s, metadata.ActionShareDownloaded, "result=key_destroyed")
	kjobDrain(t, s)
	if wrapped := kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK; wrapped != "" {
		t.Fatalf("wrapped dek remains %q", wrapped)
	}
}
