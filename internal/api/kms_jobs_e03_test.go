package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
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
		NodeID:       "node-e03",
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
	kjobActivityUser(t, s, action, needle, "")
}

func kjobActivityUser(t *testing.T, s *Server, action, needle, user string) {
	t.Helper()
	list, err := s.Meta().ListActivity(metadata.ActivityFilter{Period: "all", Limit: 300})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range list.Events {
		if ev.Action == action && strings.Contains(ev.ResourceName, needle) && (user == "" || ev.User == user) {
			if action == metadata.ActionKMSJobFinished || action == metadata.ActionKMSJobFailed {
				if !strings.Contains(ev.ResourceName, "done=") || !strings.Contains(ev.ResourceName, "total=") || !strings.Contains(ev.ResourceName, "errors=") {
					t.Fatalf("counts missing in %q", ev.ResourceName)
				}
			}
			return
		}
	}
	t.Fatalf("activity %s %q user %q missing in %d rows", action, needle, user, len(list.Events))
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
	if job.ID == "" || job.Type != kms.JobEncryptExisting || job.HolderID != "node-e03" || job.StartedBy != "admin" {
		t.Fatalf("job %#v", job)
	}
	kjobActivityUser(t, s, metadata.ActionKMSJobStarted, "bucket=jobs", "admin")
	view := kjobDo(t, s, http.MethodGet, "/api/v1/buckets/jobs/encryption", tok, nil)
	if !strings.Contains(view.Body.String(), job.ID) {
		t.Fatalf("active job missing %s", view.Body.String())
	}
	kjobDrain(t, s)
	done, err := s.Meta().GetKMSJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != kms.JobCompleted || done.Errors != 0 || done.Done != done.Total || done.Total < 3 || done.HolderID != "node-e03" {
		t.Fatalf("job result %+v", done)
	}
	kjobActivityUser(t, s, metadata.ActionKMSJobFinished, "bucket=jobs done=", "admin")
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
	kjobActivityUser(t, s, metadata.ActionKMSJobFailed, "bucket=resume", "admin")

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
	info := kjobDo(t, s, http.MethodGet, "/api/v1/public/share/"+created.Share.Token, "", nil)
	if info.Code != http.StatusForbidden || !strings.Contains(info.Body.String(), s3.MsgKeyDestroyed) {
		t.Fatalf("share info after shred %d %s", info.Code, info.Body.String())
	}
	kjobActivityHas(t, s, metadata.ActionShareDownloaded, "result=key_destroyed")
	kjobDrain(t, s)
	if wrapped := kjobObject(t, s, "jobs", "old.txt").Encryption.WrappedDEK; wrapped != "" {
		t.Fatalf("wrapped dek remains %q", wrapped)
	}
}

func TestE03DestroyRejectsOwnerUserAndOperator(t *testing.T) {
	s := newE03Server(t, "")
	admin := kjobToken(t, s)
	userTok, userID := kjobLoginUser(t, s, admin, "ownuser", auth.RoleUser)
	opTok, _ := kjobLoginUser(t, s, admin, "ownop", auth.RoleOperator)
	kjobBucket(t, s, userTok, "userbucket")
	kjobBucket(t, s, opTok, "opbucket")
	userDeny := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/userbucket/encryption/destroy", userTok, []byte(`{"confirm":"userbucket"}`))
	if userDeny.Code != http.StatusForbidden {
		t.Fatalf("owner user %d %s", userDeny.Code, userDeny.Body.String())
	}
	opDeny := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/opbucket/encryption/destroy", opTok, []byte(`{"confirm":"opbucket"}`))
	if opDeny.Code != http.StatusForbidden {
		t.Fatalf("operator %d %s", opDeny.Code, opDeny.Body.String())
	}

	kjobBucket(t, s, admin, "granted")
	kjobEnable(t, s, admin, "granted")
	grant := kjobDo(t, s, http.MethodPut, "/api/v1/buckets/granted/access", admin, []byte(fmt.Sprintf(`{"grants":[{"user_id":%q,"level":"admin","can_read":true,"can_write":true}]}`, userID)))
	if grant.Code != http.StatusOK {
		t.Fatalf("grant %d %s", grant.Code, grant.Body.String())
	}
	allowed := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/granted/encryption/destroy", userTok, []byte(`{"confirm":"granted"}`))
	if allowed.Code != http.StatusAccepted {
		t.Fatalf("bucket admin grant %d %s", allowed.Code, allowed.Body.String())
	}
}

func kjobLoginUser(t *testing.T, s *Server, adminTok, username, role string) (string, string) {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"username":%q,"password":"pass123","role":%q,"email":%q}`, username, role, username+"@test.com"))
	rec := kjobDo(t, s, http.MethodPost, "/api/v1/users", adminTok, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s %d %s", username, rec.Code, rec.Body.String())
	}
	login := kjobDo(t, s, http.MethodPost, "/api/v1/admin/login", "", []byte(fmt.Sprintf(`{"username":%q,"password":"pass123"}`, username)))
	if login.Code != http.StatusOK {
		t.Fatalf("login %s %d %s", username, login.Code, login.Body.String())
	}
	var resp struct {
		Token  string `json:"token"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("token %s %v", login.Body.String(), err)
	}
	return resp.Token, resp.UserID
}

func TestE03EncryptExistingStreams(t *testing.T) {
	s := newE03Server(t, "")
	tok := kjobToken(t, s)
	kjobBucket(t, s, tok, "stream")
	plain := strings.Repeat("x", 200*1024)
	kjobPut(t, s, "stream", "big.bin", plain)
	kjobEnable(t, s, tok, "stream")
	watch := &streamWatch{ObjectBackend: s.backend}
	s.backend = watch
	start := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/stream/encryption/rewrap", tok, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("rewrap %d %s", start.Code, start.Body.String())
	}
	kjobDrain(t, s)
	if watch.buffered {
		t.Fatalf("plaintext was buffered before the ciphertext write (%d bytes)", watch.readAtPut)
	}
	if kjobRead(t, s, "stream", "big.bin") != plain {
		t.Fatal("streamed encrypt did not round-trip")
	}
}

type streamWatch struct {
	storage.ObjectBackend
	readAtPut int64
	read      int64
	buffered  bool
}

type watchRC struct {
	io.ReadCloser
	w *streamWatch
}

func (r *watchRC) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.w.read += int64(n)
	return n, err
}

func (w *streamWatch) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	rc, info, err := w.ObjectBackend.GetObject(ctx, bucket, key)
	if err != nil {
		return rc, info, err
	}
	return &watchRC{ReadCloser: rc, w: w}, info, nil
}

func (w *streamWatch) GetObjectVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, storage.ObjectInfo, error) {
	rc, info, err := w.ObjectBackend.GetObjectVersion(ctx, bucket, key, versionID)
	if err != nil {
		return rc, info, err
	}
	return &watchRC{ReadCloser: rc, w: w}, info, nil
}

func (w *streamWatch) notePut(size int64) error {
	w.readAtPut = w.read
	if size >= 0 {
		return fmt.Errorf("ciphertext size declared %d", size)
	}
	if w.read > 64*1024 {
		w.buffered = true
	}
	return nil
}

func (w *streamWatch) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (string, error) {
	if err := w.notePut(size); err != nil {
		return "", err
	}
	return w.ObjectBackend.PutObject(ctx, bucket, key, r, size, contentType)
}

func (w *streamWatch) PutObjectVersion(ctx context.Context, bucket, key, versionID string, r io.Reader, size int64, contentType string) (string, error) {
	if err := w.notePut(size); err != nil {
		return "", err
	}
	return w.ObjectBackend.PutObjectVersion(ctx, bucket, key, versionID, r, size, contentType)
}

type vaultDeleteStub struct{ err error }

func (vaultDeleteStub) Name() string { return kms.ProviderVault }
func (vaultDeleteStub) Wrap(context.Context, string, []byte) ([]byte, int, error) {
	return nil, 0, errors.New("unused")
}
func (vaultDeleteStub) Unwrap(context.Context, string, int, []byte) ([]byte, error) {
	return nil, errors.New("unused")
}
func (vaultDeleteStub) Rewrap(context.Context, string, int, []byte) ([]byte, int, error) {
	return nil, 0, errors.New("unused")
}
func (vaultDeleteStub) Rotate(context.Context, string) (int, error) { return 0, errors.New("unused") }
func (v vaultDeleteStub) Destroy(context.Context, string) error     { return v.err }
func (vaultDeleteStub) Health(context.Context) error                { return nil }
func (vaultDeleteStub) UnwrapDEK(context.Context, string, int, int64, []byte) ([]byte, error) {
	return nil, errors.New("unused")
}
func (vaultDeleteStub) Test(context.Context) error       { return nil }
func (vaultDeleteStub) Available() bool                  { return true }
func (vaultDeleteStub) Probed() bool                     { return true }
func (vaultDeleteStub) LastError() string                { return "" }
func (vaultDeleteStub) Mark(bool, string) string         { return "" }
func (vaultDeleteStub) NoteGenerations(map[string]int64) {}
func (vaultDeleteStub) Timeout() time.Duration           { return time.Second }
func (vaultDeleteStub) PollInterval() time.Duration      { return time.Second }

func TestE03VaultDeleteErrorIsStored(t *testing.T) {
	s := newE03Server(t, "")
	tok := kjobToken(t, s)
	kjobBucket(t, s, tok, "vdel")
	kjobEnable(t, s, tok, "vdel")
	kjobPut(t, s, "vdel", "a.txt", "aaa")
	rec := kjobObject(t, s, "vdel", "a.txt")
	key, err := s.Meta().GetKMSKey(rec.Encryption.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	key.Provider = kms.ProviderVault
	if err := s.Meta().PutKMSKey(key); err != nil {
		t.Fatal(err)
	}
	s.Svc().SetVaultKMS(vaultDeleteStub{err: errors.New("deletion is not allowed")})
	resp := kjobDo(t, s, http.MethodPost, "/api/v1/buckets/vdel/encryption/destroy", tok, []byte(`{"confirm":"vdel"}`))
	if resp.Code != http.StatusAccepted || !strings.Contains(resp.Body.String(), "deletion is not allowed") {
		t.Fatalf("destroy %d %s", resp.Code, resp.Body.String())
	}
	var job metadata.KMSJob
	if err := json.Unmarshal(resp.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.VaultDeleteError == "" || !strings.Contains(strings.Join(job.Samples, "\n"), "deletion is not allowed") {
		t.Fatalf("job did not keep the vault error %+v", job)
	}
	stored, err := s.Meta().GetKMSKey(key.ID)
	if err != nil || stored.Status != "destroyed" {
		t.Fatalf("key %+v %v", stored, err)
	}
}
