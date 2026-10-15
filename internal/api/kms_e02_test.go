package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestE02IdleHasNoVaultClient(t *testing.T) {
	t.Setenv("STORAGE_METRICS_TOKEN", "")
	s := testServer(t)
	if s.Svc().VaultKMS() != nil {
		t.Fatal("vault client allocated without config")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.StartBackground(ctx)
	cancel()
	if s.Svc().VaultKMS() != nil {
		t.Fatal("background start allocated a vault client")
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") || !strings.Contains(rec.Body.String(), `"kms"`) {
		t.Fatalf("healthz %s", rec.Body.String())
	}
	var body struct {
		KMS struct {
			Provider  string `json:"provider"`
			Available bool   `json:"available"`
		} `json:"kms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.KMS.Provider != kms.ProviderLocal || !body.KMS.Available {
		t.Fatalf("kms health %+v", body.KMS)
	}
	metrics := httptest.NewRecorder()
	s.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || strings.Contains(metrics.Body.String(), "datasafe_kms") {
		t.Fatalf("idle metrics %d %s", metrics.Code, metrics.Body.String())
	}
}

func TestE02IdlePostgresNoVaultClient(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	t.Setenv("STORAGE_METRICS_TOKEN", "")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_e02_%d", time.Now().UnixNano())
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
	bg, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartBackground(bg)
	if s.Svc().VaultKMS() != nil {
		t.Fatal("postgres idle allocated a vault client")
	}
	time.Sleep(2500 * time.Millisecond)
	if s.Svc().VaultKMS() != nil {
		t.Fatal("settings watcher opened a vault client with no provider")
	}
	metrics := httptest.NewRecorder()
	s.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || strings.Contains(metrics.Body.String(), "datasafe_kms") {
		t.Fatalf("idle metrics %d %s", metrics.Code, metrics.Body.String())
	}
}

func TestE02SettingsNeverReturnSecrets(t *testing.T) {
	secret := "super-secret-id-e02"
	token := "hvs.should-not-leak"
	fake := &apiFakeVault{secret: secret, token: "hvs.node", plain: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	dead := "dead-secret-e02-not-stored"
	refused := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/kms/settings", tok, []byte(`{"addr":"http://127.0.0.1:1","auth":"approle","role_id":"role-e02","secret_id":"`+dead+`","transit_mount":"transit"}`), "")
	if refused.Code != http.StatusRequestTimeout || strings.Contains(refused.Body.String(), dead) {
		t.Fatalf("bogus addr %d %s", refused.Code, refused.Body.String())
	}
	if stored, err := s.Meta().GetSystemConfig(); err != nil || stored.KMS.SecretID != "" || stored.KMS.Addr != "" {
		t.Fatalf("failed probe was stored: %+v %v", stored.KMS, err)
	}

	body := []byte(`{"addr":"` + srv.URL + `","auth":"approle","role_id":"role-e02","secret_id":"` + secret + `","token":"` + token + `","transit_mount":"transit"}`)
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/kms/settings", tok, body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("settings response leaked a secret: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"secret_id_set":true`) {
		t.Fatalf("secret not marked set: %s", rec.Body.String())
	}
	fake.mu.Lock()
	deleted := fake.deletes
	fake.mu.Unlock()
	if deleted != 0 {
		t.Fatalf("connection test deleted a key: %d", deleted)
	}
	assertNoKMSSecret(t, s, tok, secret, token)
	stored, err := s.Meta().GetSystemConfig()
	if err != nil || stored.KMS.SecretID != secret || stored.KMS.Token != token {
		t.Fatalf("stored secret missing: %+v %v", stored.KMS, err)
	}
	hz := httptest.NewRecorder()
	s.Handler().ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if !strings.Contains(hz.Body.String(), `"available":true`) {
		t.Fatalf("healthz after a successful probe: %s", hz.Body.String())
	}

	bogus := "nope-secret-e02"
	bad := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/kms/settings", tok, []byte(`{"addr":"`+srv.URL+`","auth":"approle","role_id":"role-e02","secret_id":"`+bogus+`","transit_mount":"transit"}`), "")
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "Неверный Secret ID") || strings.Contains(bad.Body.String(), bogus) {
		t.Fatalf("bogus save %d %s", bad.Code, bad.Body.String())
	}
	stored, err = s.Meta().GetSystemConfig()
	if err != nil || stored.KMS.SecretID != secret {
		t.Fatalf("bogus save replaced the secret: %+v %v", stored.KMS, err)
	}
	injected := "injected-system-secret-e02"
	sys := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/system", tok, []byte(`{"trash_retention_days":30,"kms":{"secret_id":"`+injected+`","token":"`+token+`"}}`), "")
	if sys.Code != http.StatusOK {
		t.Fatalf("system put %d %s", sys.Code, sys.Body.String())
	}
	assertNoKMSSecret(t, s, tok, secret, token)
	assertNoKMSSecret(t, s, tok, injected, bogus)
	stored, err = s.Meta().GetSystemConfig()
	if err != nil || stored.KMS.SecretID != secret {
		t.Fatalf("system put changed kms: %+v %v", stored.KMS, err)
	}
}

func assertNoKMSSecret(t *testing.T, s *api.Server, tok, secret, token string) {
	t.Helper()
	paths := []string{
		"/api/v1/settings/system",
		"/api/v1/kms/settings",
		"/api/v1/kms/nodes",
		"/api/v1/settings/buckets",
		"/api/v1/settings/security-status",
		"/api/v1/settings/mfa-policy",
		"/api/v1/settings/sharing",
		"/api/v1/activity",
		"/api/v1/activity/export?format=json&period=all",
		"/api/v1/activity/export?format=csv&period=all",
		"/healthz",
		"/api/v1/health",
	}
	for _, path := range paths {
		rec := doJSON(t, s.Handler(), http.MethodGet, path, tok, nil, "")
		if rec.Code >= 500 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), secret) || (token != "" && strings.Contains(rec.Body.String(), token)) {
			t.Fatalf("%s leaked: %s", path, rec.Body.String())
		}
	}
}

func TestE02WatchVaultConfig(t *testing.T) {
	s := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartBackground(ctx)
	time.Sleep(200 * time.Millisecond)
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.KMS = metadata.VaultKMSConfig{
		Provider: kms.ProviderVault,
		Addr:     "http://127.0.0.1:1",
		Auth:     "approle",
		RoleID:   "role",
		SecretID: "s",
	}
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if s.Svc().VaultKMS() != nil {
		t.Fatal("bolt started a follower watcher")
	}
	go s.WatchVaultConfig(ctx, 30*time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Svc().VaultKMS() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("follower did not load vault settings within 1s")
}

func TestE02HealthFollowsProbe(t *testing.T) {
	fake := &apiFakeVault{secret: "s3cret", token: "hvs.node", plain: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/kms/settings", tok, []byte(`{"addr":"`+srv.URL+`","auth":"approle","role_id":"role","secret_id":"s3cret","transit_mount":"transit"}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	waitKMSAvailable(t, s, true)
	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()
	waitKMSAvailable(t, s, false)
	fake.mu.Lock()
	fake.down = false
	fake.mu.Unlock()
	waitKMSAvailable(t, s, true)
}

func waitKMSAvailable(t *testing.T, s *api.Server, want bool) {
	t.Helper()
	deadline := time.Now().Add(7 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		last = rec.Body.String()
		var body struct {
			KMS struct {
				Available bool `json:"available"`
			} `json:"kms"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil && body.KMS.Available == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("kms.available want %v last %s", want, last)
}

func TestE02VaultSSEKMSFailClosed(t *testing.T) {
	fake := &apiFakeVault{secret: "s3cret", token: "hvs.node", plain: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	p, err := vaulttransit.Open(vaulttransit.Config{
		Provider: kms.ProviderVault,
		Addr:     srv.URL,
		RoleID:   "role",
		SecretID: "s3cret",
		Timeout:  time.Second,
		CacheTTL: 2 * time.Second,
	})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	s := testServer(t)
	s.SetVaultKMS(p)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "kmsbox", nil)
	createBucket(t, s, tok, "plainbox", nil)
	createBucket(t, s, tok, "aesbox", nil)

	view := doJSON(t, s.Handler(), http.MethodGet, "/api/v1/buckets/kmsbox/encryption", tok, nil, "")
	if !strings.Contains(view.Body.String(), `"vault_configured":true`) {
		t.Fatalf("view %s", view.Body.String())
	}
	putEnc := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/kmsbox/encryption", tok, []byte(`{"enabled":true,"provider":"vault-transit"}`), "")
	if putEnc.Code != http.StatusOK || !strings.Contains(putEnc.Body.String(), `"aws:kms"`) {
		t.Fatalf("enable vault %d %s", putEnc.Code, putEnc.Body.String())
	}
	obj := s3Call(t, s, http.MethodPut, "/kmsbox/a.txt", []byte("vault-body"), nil)
	if obj.Code != http.StatusOK {
		t.Fatalf("put %d %s", obj.Code, obj.Body.String())
	}
	got := s3Call(t, s, http.MethodGet, "/kmsbox/a.txt", nil, nil)
	if got.Code != http.StatusOK || got.Body.String() != "vault-body" {
		t.Fatalf("get %d %q", got.Code, got.Body.String())
	}

	fake.mu.Lock()
	calls := fake.calls
	fake.mu.Unlock()
	plain := s3Call(t, s, http.MethodPut, "/plainbox/p.txt", []byte("plain"), nil)
	if plain.Code != http.StatusOK {
		t.Fatalf("plain put %d %s", plain.Code, plain.Body.String())
	}
	aes := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/aesbox/encryption", tok, []byte(`{"enabled":true,"provider":"local"}`), "")
	if aes.Code != http.StatusOK {
		t.Fatalf("aes %d %s", aes.Code, aes.Body.String())
	}
	aesPut := s3Call(t, s, http.MethodPut, "/aesbox/b.txt", []byte("local-body"), nil)
	if aesPut.Code != http.StatusOK {
		t.Fatalf("aes put %d %s", aesPut.Code, aesPut.Body.String())
	}
	fake.mu.Lock()
	if fake.calls != calls {
		t.Fatalf("plaintext or AES256 called vault: %d -> %d", calls, fake.calls)
	}
	fake.mu.Unlock()
	bad := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/kms/test", tok, []byte(`{"addr":"`+srv.URL+`","auth":"approle","role_id":"role","secret_id":"nope"}`), "")
	if bad.Code != http.StatusBadGateway || !strings.Contains(bad.Body.String(), "Неверный Secret ID") || strings.Contains(bad.Body.String(), "nope") {
		t.Fatalf("bad secret %d %s", bad.Code, bad.Body.String())
	}
	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()

	cached := s3Call(t, s, http.MethodGet, "/kmsbox/a.txt", nil, nil)
	if cached.Code != http.StatusOK || cached.Body.String() != "vault-body" {
		t.Fatalf("cached get %d %q", cached.Code, cached.Body.String())
	}
	blocked := s3Call(t, s, http.MethodPut, "/kmsbox/new.txt", []byte("nope"), nil)
	if blocked.Code != http.StatusServiceUnavailable || !strings.Contains(blocked.Body.String(), "KMS unavailable") {
		t.Fatalf("put while down %d %s", blocked.Code, blocked.Body.String())
	}
	plainWhileDown := s3Call(t, s, http.MethodGet, "/plainbox/p.txt", nil, nil)
	if plainWhileDown.Code != http.StatusOK || plainWhileDown.Body.String() != "plain" {
		t.Fatalf("plain get %d %q", plainWhileDown.Code, plainWhileDown.Body.String())
	}
	time.Sleep(2500 * time.Millisecond)
	miss := s3Call(t, s, http.MethodGet, "/kmsbox/a.txt", nil, nil)
	if miss.Code != http.StatusServiceUnavailable || !strings.Contains(miss.Body.String(), "KMS unavailable") {
		t.Fatalf("get after ttl %d %s", miss.Code, miss.Body.String())
	}
	console := doJSON(t, s.Handler(), http.MethodGet, "/api/v1/buckets/kmsbox/objects/a.txt", tok, nil, "")
	if console.Code != http.StatusServiceUnavailable || !strings.Contains(console.Body.String(), "KMS unavailable") {
		t.Fatalf("console get %d %s", console.Code, console.Body.String())
	}
	fake.mu.Lock()
	if fake.deletes != 0 {
		t.Fatalf("failed connection test deleted a key: %d", fake.deletes)
	}
	fake.mu.Unlock()
}

type apiFakeVault struct {
	mu      sync.Mutex
	secret  string
	token   string
	down    bool
	plain   map[string][]byte
	calls   int
	deletes int
}

func (f *apiFakeVault) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls++
	down := f.down
	secret := f.secret
	token := f.token
	f.mu.Unlock()
	if down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	switch {
	case r.URL.Path == "/v1/auth/approle/login":
		var req struct {
			SecretID string `json:"secret_id"`
		}
		_ = json.Unmarshal(body, &req)
		if req.SecretID != secret {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["invalid secret id"]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": token, "lease_duration": 3600}})
	case r.URL.Path == "/v1/sys/health":
		w.WriteHeader(http.StatusOK)
	case strings.Contains(r.URL.Path, "/encrypt/"):
		var req struct {
			Plaintext string `json:"plaintext"`
		}
		_ = json.Unmarshal(body, &req)
		raw, _ := base64.StdEncoding.DecodeString(req.Plaintext)
		ct := "vault:v1:" + base64.StdEncoding.EncodeToString(raw)
		f.mu.Lock()
		f.plain[ct] = append([]byte(nil), raw...)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ciphertext": ct, "key_version": 1}})
	case strings.Contains(r.URL.Path, "/decrypt/"):
		var req struct {
			Ciphertext string `json:"ciphertext"`
		}
		_ = json.Unmarshal(body, &req)
		f.mu.Lock()
		raw := append([]byte(nil), f.plain[req.Ciphertext]...)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"plaintext": base64.StdEncoding.EncodeToString(raw)}})
	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "datasafe-connection-test"):
		f.mu.Lock()
		f.deletes++
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
