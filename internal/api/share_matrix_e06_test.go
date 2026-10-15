package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
)

// E06 sharing × encryption matrix. Columns are separate buckets so a destroyed
// key or a sharing disable does not change the other cells. Anonymous SSE-KMS
// stays 403 even when Vault is down or the key is destroyed: that check runs
// before unwrap.
func TestE06ShareMatrixBolt(t *testing.T) {
	runE06ShareMatrix(t, testServer(t))
}

func TestE06ShareMatrixPostgres(t *testing.T) {
	s := e06PostgresServer(t)
	runE06ShareMatrix(t, s)
}

func e06PostgresServer(t *testing.T) *api.Server {
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
	name := fmt.Sprintf("ds_e06_%d", time.Now().UnixNano())
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
		SSEMasterKey: "test-sse-master-key",
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
	return s
}

type e06Col struct {
	name    string
	kind    string // plain, s3, kms
	body    string
	token   string
	linkID  string
	presign string
}

func runE06ShareMatrix(t *testing.T, s *api.Server) {
	t.Helper()
	fake := &apiFakeVault{secret: "s3cret", token: "hvs.e06", plain: map[string][]byte{}}
	vault := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(vault.Close)
	p, err := vaulttransit.Open(vaulttransit.Config{
		Provider: "vault-transit",
		Addr:     vault.URL,
		RoleID:   "role",
		SecretID: "s3cret",
		Timeout:  time.Second,
		CacheTTL: 40 * time.Millisecond,
	})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	s.SetVaultKMS(p)
	putShareWebhook(t, s)
	tok := loginToken(t, s, "admin", "admin")
	stranger := createUser(t, s, tok, "e06-stranger", auth.RoleUser)

	cols := []*e06Col{
		{name: "e06plain", kind: "plain"},
		{name: "e06s3", kind: "s3"},
		{name: "e06up", kind: "kms"},
		{name: "e06down", kind: "kms"},
		{name: "e06dead", kind: "kms"},
	}
	for _, col := range cols {
		col.body = "body-" + col.name
		e06Prepare(t, s, tok, col)
	}

	for _, col := range cols {
		e06AssertOpen(t, s, col)
	}

	e06BPA(t, s, tok, cols[0], cols[1])

	for _, col := range cols {
		rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/"+col.name+"/shares", stranger, []byte(`{"key":"a.txt","expires_in_sec":600}`), "")
		e06HTTP(t, rec, http.StatusForbidden, "forbidden", col.name+" stranger share")
		e06Activity(t, s, metadata.ActionShareBlocked, col.name+" result=forbidden")
		e06Outbox(t, s, "ShareBlocked", col.name, "forbidden", false)
	}

	for _, col := range cols {
		off := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/"+col.name+"/sharing", tok, []byte(`{"share_links_allowed":false}`), "")
		e06HTTP(t, off, http.StatusOK, "", col.name+" disable sharing")
		dl := httptest.NewRecorder()
		s.Handler().ServeHTTP(dl, httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+col.token+"/download", nil))
		e06HTTP(t, dl, http.StatusForbidden, s3.ShareDisabledMessage, col.name+" share while disabled")
		e06Outbox(t, s, "ShareBlocked", col.name, "", true)
		e06Activity(t, s, metadata.ActionShareBlocked, col.linkID)
		replay := e06Replay(t, s, col.presign)
		e06HTTP(t, replay, http.StatusForbidden, s3.ShareDisabledMessage, col.name+" presign while disabled")
		e06Activity(t, s, metadata.ActionShareBlocked, col.name+" result=presign_blocked")
		e06Outbox(t, s, "ShareBlocked", col.name, "presign_blocked", false)
		issued := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/presign", tok, []byte(`{"method":"GET","bucket":"`+col.name+`","key":"a.txt","expires_seconds":300}`), "")
		e06HTTP(t, issued, http.StatusForbidden, "sharing_disabled", col.name+" issue presign while disabled")
		on := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/"+col.name+"/sharing", tok, []byte(`{"share_links_allowed":true}`), "")
		e06HTTP(t, on, http.StatusOK, "", col.name+" enable sharing")
	}

	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()
	time.Sleep(300 * time.Millisecond)

	for _, col := range cols {
		if col.kind != "kms" {
			anon := e06Anon(t, s, col)
			e06HTTP(t, anon, http.StatusOK, col.body, col.name+" anon while vault down")
			continue
		}
		anon := e06Anon(t, s, col)
		e06HTTP(t, anon, http.StatusForbidden, s3.MsgKMSPublic, col.name+" anon kms while vault down")
		e06Activity(t, s, metadata.ActionObjectDownloaded, col.name+"/a.txt", "result=kms_public")
		e06Outbox(t, s, "ObjectDownloaded", col.name, "kms_public", false)
	}
	down := cols[3]
	downShare := httptest.NewRecorder()
	s.Handler().ServeHTTP(downShare, httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+down.token+"/download", nil))
	e06HTTP(t, downShare, http.StatusServiceUnavailable, "KMS unavailable", "vault down share")
	e06Activity(t, s, metadata.ActionShareDownloaded, "result=kms_unavailable")
	e06Outbox(t, s, "ShareDownloaded", down.name, "kms_unavailable", true)
	downPre := e06Replay(t, s, down.presign)
	e06HTTP(t, downPre, http.StatusServiceUnavailable, "KMS unavailable", "vault down presign")
	e06Activity(t, s, metadata.ActionObjectDownloaded, down.name+"/a.txt", "result=kms_unavailable")
	e06Outbox(t, s, "ShareDownloaded", down.name, "kms_unavailable", false)

	dead := cols[4]
	e06DestroyKey(t, s, dead.name)
	deadShare := httptest.NewRecorder()
	s.Handler().ServeHTTP(deadShare, httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+dead.token+"/download", nil))
	e06HTTP(t, deadShare, http.StatusForbidden, s3.MsgKeyDestroyed, "destroyed share")
	e06Activity(t, s, metadata.ActionShareDownloaded, "result=key_destroyed")
	e06Outbox(t, s, "ShareDownloaded", dead.name, "key_destroyed", true)
	deadPre := e06Replay(t, s, dead.presign)
	e06HTTP(t, deadPre, http.StatusForbidden, s3.MsgKeyDestroyed, "destroyed presign")
	e06Activity(t, s, metadata.ActionObjectDownloaded, "result=key_destroyed")
	e06Outbox(t, s, "ShareDownloaded", dead.name, "key_destroyed", false)
	deadAnon := e06Anon(t, s, dead)
	e06HTTP(t, deadAnon, http.StatusForbidden, s3.MsgKMSPublic, "destroyed anon still kms-public")
}

func e06Prepare(t *testing.T, s *api.Server, tok string, col *e06Col) {
	t.Helper()
	createBucket(t, s, tok, col.name, nil)
	headers := map[string]string{}
	switch col.kind {
	case "s3":
		headers["x-amz-server-side-encryption"] = "AES256"
	case "kms":
		headers["x-amz-server-side-encryption"] = "aws:kms"
	}
	put := s3Call(t, s, http.MethodPut, "/"+col.name+"/a.txt", []byte(col.body), headers)
	e06HTTP(t, put, http.StatusOK, "", col.name+" put")
	pub := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/buckets/"+col.name, tok, []byte(`{"visibility":"public-read"}`), "")
	e06HTTP(t, pub, http.StatusOK, "", col.name+" public-read")
	share := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/"+col.name+"/shares", tok, []byte(`{"key":"a.txt","expires_in_sec":3600}`), "")
	e06HTTP(t, share, http.StatusCreated, "", col.name+" share")
	var created struct {
		Share struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"share"`
	}
	if err := json.Unmarshal(share.Body.Bytes(), &created); err != nil || created.Share.Token == "" || created.Share.ID == "" {
		t.Fatalf("%s share token %s", col.name, share.Body.String())
	}
	col.token = created.Share.Token
	col.linkID = created.Share.ID
	e06Outbox(t, s, "ShareCreated", col.name, "", true)
	pre := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/presign", tok, []byte(`{"method":"GET","bucket":"`+col.name+`","key":"a.txt","expires_seconds":300}`), "")
	e06HTTP(t, pre, http.StatusOK, "http", col.name+" presign")
	var issued struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(pre.Body.Bytes(), &issued); err != nil || issued.URL == "" {
		t.Fatalf("%s presign %s", col.name, pre.Body.String())
	}
	col.presign = issued.URL
	e06Activity(t, s, metadata.ActionPresignIssued, col.name+"/a.txt")
	e06Outbox(t, s, "PresignIssued", col.name, "", false)
}

func e06AssertOpen(t *testing.T, s *api.Server, col *e06Col) {
	t.Helper()
	anon := e06Anon(t, s, col)
	if col.kind == "kms" {
		e06HTTP(t, anon, http.StatusForbidden, s3.MsgKMSPublic, col.name+" anon kms")
		e06Activity(t, s, metadata.ActionObjectDownloaded, col.name+"/a.txt", "result=kms_public")
		e06Outbox(t, s, "ObjectDownloaded", col.name, "kms_public", false)
	} else {
		e06HTTP(t, anon, http.StatusOK, col.body, col.name+" anon")
		e06Activity(t, s, metadata.ActionObjectDownloaded, col.name+"/a.txt")
		e06Outbox(t, s, "ObjectDownloaded", col.name, "ok", false)
	}
	replay := e06Replay(t, s, col.presign)
	e06HTTP(t, replay, http.StatusOK, col.body, col.name+" presign get")
	if col.kind != "plain" {
		e06Activity(t, s, metadata.ActionObjectDownloaded, col.name+"/a.txt key_id=")
		e06Outbox(t, s, "ShareDownloaded", col.name, "", false)
	}
	dl := httptest.NewRecorder()
	s.Handler().ServeHTTP(dl, httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+col.token+"/download", nil))
	e06HTTP(t, dl, http.StatusOK, col.body, col.name+" share")
	e06Activity(t, s, metadata.ActionShareDownloaded, col.linkID)
	if col.kind == "kms" {
		e06Activity(t, s, metadata.ActionShareDownloaded, col.linkID+" key_id=")
		e06OutboxKey(t, s, "ShareDownloaded", col.name)
	} else {
		e06Outbox(t, s, "ShareDownloaded", col.name, "", true)
	}
}

func e06BPA(t *testing.T, s *api.Server, tok string, plain, s3col *e06Col) {
	t.Helper()
	on := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/sharing", tok, []byte(`{"public_access_blocked":true}`), "")
	e06HTTP(t, on, http.StatusOK, "", "bpa on")
	for _, col := range []*e06Col{plain, s3col} {
		anon := e06Anon(t, s, col)
		e06HTTP(t, anon, http.StatusForbidden, "public access is blocked by policy", col.name+" bpa")
		e06Activity(t, s, metadata.ActionObjectDownloaded, col.name+"/a.txt", "result=public_blocked")
		e06Outbox(t, s, "ObjectDownloaded", col.name, "public_blocked", false)
	}
	off := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/sharing", tok, []byte(`{"public_access_blocked":false}`), "")
	e06HTTP(t, off, http.StatusOK, "", "bpa off")
}

func e06DestroyKey(t *testing.T, s *api.Server, bucket string) {
	t.Helper()
	sk := storageKeyOf(t, s, bucket)
	meta, err := s.Meta().GetObject(sk, "a.txt")
	if err != nil || meta.Encryption == nil || meta.Encryption.KeyID == "" {
		t.Fatalf("%s encryption meta %+v %v", bucket, meta.Encryption, err)
	}
	key, err := s.Meta().GetKMSKey(meta.Encryption.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	key.Status = "destroyed"
	if err := s.Meta().PutKMSKey(key); err != nil {
		t.Fatal(err)
	}
}

func e06Anon(t *testing.T, s *api.Server, col *e06Col) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+col.name+"/a.txt", nil))
	return rec
}

func e06Replay(t *testing.T, s *api.Server, raw string) *httptest.ResponseRecorder {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, u.RequestURI(), nil)
	req.Host = u.Host
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func e06HTTP(t *testing.T, rec *httptest.ResponseRecorder, want int, substr, what string) {
	t.Helper()
	if rec.Code != want || (substr != "" && !strings.Contains(rec.Body.String(), substr)) {
		t.Fatalf("%s: got %d %s", what, rec.Code, rec.Body.String())
	}
}

func e06Activity(t *testing.T, s *api.Server, action string, parts ...string) {
	t.Helper()
	res, err := s.Meta().ListActivity(metadata.ActivityFilter{Action: action, Limit: 400})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range res.Events {
		ok := true
		for _, part := range parts {
			if !strings.Contains(ev.ResourceName, part) {
				ok = false
				break
			}
		}
		if ok {
			return
		}
	}
	t.Fatalf("activity %s missing %q (%d events)", action, parts, res.Total)
}

func e06Outbox(t *testing.T, s *api.Server, eventType, bucket, result string, needLink bool) {
	t.Helper()
	for _, row := range e06OutboxRows(t, s) {
		if row.EventType != eventType || row.Bucket != bucket {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if result != "" && payload["result"] != result {
			continue
		}
		if needLink && payload["link_id"] == "" {
			continue
		}
		if _, ok := row.Attempts["webhook"]; !ok {
			t.Fatalf("%s %s has no webhook attempt", eventType, bucket)
		}
		return
	}
	t.Fatalf("outbox %s bucket=%s result=%q link=%v missing", eventType, bucket, result, needLink)
}

func e06OutboxKey(t *testing.T, s *api.Server, eventType, bucket string) {
	t.Helper()
	for _, row := range e06OutboxRows(t, s) {
		if row.EventType != eventType || row.Bucket != bucket {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["key_id"] == "" || payload["key_id"] == nil {
			continue
		}
		return
	}
	t.Fatalf("outbox %s bucket=%s missing key_id", eventType, bucket)
}

func e06OutboxRows(t *testing.T, s *api.Server) []events.OutboxRecord {
	t.Helper()
	st, ok := s.Meta().(events.OutboxStore)
	if !ok {
		t.Fatal("metadata store has no outbox")
	}
	rows, err := st.ListDueOutbox(context.Background(), time.Now().Add(time.Minute), 500)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
