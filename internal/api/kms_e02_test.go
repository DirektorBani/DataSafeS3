package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
)

func TestE02IdleHasNoVaultClient(t *testing.T) {
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
}

func TestE02SettingsNeverReturnSecrets(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	secret := "super-secret-id-e02"
	token := "hvs.should-not-leak"
	body := []byte(`{"addr":"http://127.0.0.1:1","auth":"approle","role_id":"role-e02","secret_id":"` + secret + `","token":"` + token + `","transit_mount":"transit"}`)
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
	got := doJSON(t, s.Handler(), http.MethodGet, "/api/v1/kms/settings", tok, nil, "")
	if strings.Contains(got.Body.String(), secret) || strings.Contains(got.Body.String(), token) {
		t.Fatalf("get leaked: %s", got.Body.String())
	}
	hz := httptest.NewRecorder()
	s.Handler().ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if strings.Contains(hz.Body.String(), secret) || strings.Contains(hz.Body.String(), token) {
		t.Fatalf("healthz leaked: %s", hz.Body.String())
	}
	stored, err := s.Meta().GetSystemConfig()
	if err != nil || stored.KMS.SecretID != secret {
		t.Fatalf("stored secret missing: %+v %v", stored.KMS, err)
	}
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
}

type apiFakeVault struct {
	mu     sync.Mutex
	secret string
	token  string
	down   bool
	plain  map[string][]byte
	calls  int
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
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
