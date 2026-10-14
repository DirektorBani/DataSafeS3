package vaulttransit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type fakeVault struct {
	mu       sync.Mutex
	secret   string
	token    string
	down     bool
	forbid   bool
	echo     bool
	plain    map[string][]byte
	decrypts int
}

func (f *fakeVault) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	down, forbid, echo, secret, token := f.down, f.forbid, f.echo, f.secret, f.token
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
			_, _ = w.Write([]byte(`{"errors":["invalid secret id ` + req.SecretID + `"]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": token, "lease_duration": 3600}})
	case r.URL.Path == "/v1/auth/token/renew-self":
		_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"lease_duration": 3600}})
	case r.URL.Path == "/v1/sys/health":
		_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
	case strings.Contains(r.URL.Path, "/encrypt/"):
		if forbid {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
			return
		}
		if echo {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":["leaked ` + r.Header.Get("X-Vault-Token") + `"]}`))
			return
		}
		var req struct {
			Plaintext string `json:"plaintext"`
		}
		_ = json.Unmarshal(body, &req)
		raw, _ := base64.StdEncoding.DecodeString(req.Plaintext)
		ct := "vault:v1:" + base64.StdEncoding.EncodeToString(raw)
		f.mu.Lock()
		if f.plain == nil {
			f.plain = map[string][]byte{}
		}
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
		f.decrypts++
		f.mu.Unlock()
		if raw == nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["cipher not found"]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"plaintext": base64.StdEncoding.EncodeToString(raw)}})
	case strings.Contains(r.URL.Path, "/rewrap/"):
		var req struct {
			Ciphertext string `json:"ciphertext"`
		}
		_ = json.Unmarshal(body, &req)
		f.mu.Lock()
		raw := append([]byte(nil), f.plain[req.Ciphertext]...)
		f.mu.Unlock()
		ct := "vault:v2:" + base64.StdEncoding.EncodeToString(raw)
		f.mu.Lock()
		f.plain[ct] = raw
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ciphertext": ct, "key_version": 2}})
	case strings.Contains(r.URL.Path, "/keys/"):
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func testProvider(t *testing.T, srv *httptest.Server, secret string, ttl time.Duration) *Provider {
	t.Helper()
	p, err := Open(Config{
		Provider: kms.ProviderVault,
		Addr:     srv.URL,
		RoleID:   "role-1",
		SecretID: secret,
		Timeout:  time.Second,
		CacheTTL: ttl,
		CacheMax: 8,
	})
	if err != nil || p == nil {
		t.Fatalf("open: %v", err)
	}
	return p
}

func TestTransitRoundTripCacheAndScrub(t *testing.T) {
	fake := &fakeVault{secret: "good-secret", token: "hvs.SUPERSECRETTOKEN", plain: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	p := testProvider(t, srv, "good-secret", time.Hour)
	ctx := t.Context()

	wrapped, ver, err := p.Wrap(ctx, "datasafe-default-box", []byte("dek-bytes-0123456789abcdef0123"))
	if err != nil || ver != 1 || !strings.HasPrefix(string(wrapped), "vault:v1:") {
		t.Fatalf("wrap %q ver %d err %v", wrapped, ver, err)
	}
	dek, err := p.UnwrapDEK(ctx, "datasafe-default-box", ver, 1, wrapped)
	if err != nil || string(dek) != "dek-bytes-0123456789abcdef0123" {
		t.Fatalf("unwrap %q %v", dek, err)
	}
	fake.mu.Lock()
	first := fake.decrypts
	fake.mu.Unlock()
	if _, err := p.UnwrapDEK(ctx, "datasafe-default-box", ver, 1, wrapped); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	if fake.decrypts != first {
		t.Fatalf("cache miss: decrypts %d want %d", fake.decrypts, first)
	}
	fake.mu.Unlock()

	p.NoteGenerations(map[string]int64{"datasafe-default-box": 2})
	if _, err := p.UnwrapDEK(ctx, "datasafe-default-box", ver, 2, wrapped); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	if fake.decrypts != first+1 {
		t.Fatalf("generation did not drop cache: %d", fake.decrypts)
	}
	fake.down = true
	fake.mu.Unlock()
	if _, err := p.UnwrapDEK(ctx, "datasafe-default-box", ver, 2, wrapped); err != nil {
		t.Fatal("cache hit while vault is down:", err)
	}

	fake.mu.Lock()
	fake.echo = true
	fake.down = false
	fake.mu.Unlock()
	_, _, err = p.client.Encrypt(ctx, "datasafe-default-box", []byte("x"))
	if err == nil || strings.Contains(err.Error(), "SUPERSECRETTOKEN") {
		t.Fatalf("token leaked or missing error: %v", err)
	}
}

func TestTransitBadSecretPermissionAndTimeout(t *testing.T) {
	fake := &fakeVault{secret: "good-secret", token: "hvs.ok", plain: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)
	p := testProvider(t, srv, "wrong-secret", time.Minute)
	rawErr := p.client.TestLoginEncrypt(t.Context())
	if rawErr == nil || strings.Contains(rawErr.Error(), "wrong-secret") {
		t.Fatalf("bad secret leaked or missing: %v", rawErr)
	}
	if Classify(rawErr) != ErrBadSecret {
		t.Fatalf("bad secret class: %v", rawErr)
	}

	okp := testProvider(t, srv, "good-secret", time.Minute)
	fake.mu.Lock()
	fake.forbid = true
	fake.mu.Unlock()
	err := okp.Test(t.Context())
	if err != ErrNoPermission {
		t.Fatalf("permission: %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)
	tp, err := Open(Config{
		Provider: kms.ProviderVault, Addr: slow.URL, RoleID: "r", SecretID: "s", Timeout: 50 * time.Millisecond,
	})
	if err != nil || tp == nil {
		t.Fatal(err)
	}
	if Classify(tp.Test(context.Background())) != ErrTimeout {
		t.Fatal("expected timeout")
	}
}

func TestStaticTokenKeptWhenNotRenewable(t *testing.T) {
	var deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/auth/token/renew-self":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["token not renewable"]}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "datasafe-connection-test"):
			deletes++
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/keys/"):
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/encrypt/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ciphertext": "vault:v1:YQ==", "key_version": 1}})
		case strings.Contains(r.URL.Path, "/decrypt/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"plaintext": base64.StdEncoding.EncodeToString([]byte("datasafe"))}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	p, err := Open(Config{Provider: kms.ProviderVault, Addr: srv.URL, Token: "root", Timeout: time.Second})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	if err := p.Test(t.Context()); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatalf("connection-test key deletes %d", deletes)
	}

	var calls int
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/auth/token/renew-self" {
			t.Errorf("expired token was used for %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["token is expired"]}`))
	}))
	t.Cleanup(expired.Close)
	bad, err := Open(Config{Provider: kms.ProviderVault, Addr: expired.URL, Token: "dead", Timeout: time.Second})
	if err != nil || bad == nil {
		t.Fatal(err)
	}
	if err := bad.Test(t.Context()); err == nil {
		t.Fatal("expired token was accepted")
	}
	if calls != 1 {
		t.Fatalf("renew calls %d", calls)
	}
}

func TestHealthAcceptsStandby(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusTooManyRequests, 472, 473} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/sys/health":
					w.WriteHeader(code)
				case "/v1/auth/token/renew-self":
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"errors":["token not renewable"]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			p, err := Open(Config{Provider: kms.ProviderVault, Addr: srv.URL, Token: "root", Timeout: time.Second})
			if err != nil || p == nil {
				t.Fatal(err)
			}
			if err := p.Health(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
	sealed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(sealed.Close)
	p, err := Open(Config{Provider: kms.ProviderVault, Addr: sealed.URL, Token: "root", Timeout: time.Second})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	if err := p.Health(t.Context()); err == nil {
		t.Fatal("sealed vault was available")
	}
}

func TestDisabledConfigDoesNotOpen(t *testing.T) {
	p, err := Open(Config{})
	if err != nil || p != nil {
		t.Fatalf("disabled open: %+v %v", p, err)
	}
	p, err = Open(Config{Provider: kms.ProviderVault})
	if err != nil || p != nil {
		t.Fatalf("addr required: %+v %v", p, err)
	}
}

func TestMergeEnvOverridesSharedSecret(t *testing.T) {
	t.Setenv("STORAGE_KMS_PROVIDER", "vault-transit")
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "http://vault.example:8200")
	t.Setenv("STORAGE_KMS_VAULT_SECRET_ID", "from-env")
	env, locks := LoadEnv()
	merged := Merge(env, locks, metadata.VaultKMSConfig{
		Provider: kms.ProviderVault,
		Addr:     "http://other:8200",
		SecretID: "from-console",
		Auth:     "approle",
		RoleID:   "console-role",
	})
	if merged.Addr != "http://vault.example:8200" || merged.SecretID != "from-env" {
		t.Fatalf("%+v", merged)
	}
	if !locks.Addr || !locks.Secret || !locks.Provider {
		t.Fatalf("locks %+v", locks)
	}
	if merged.RoleID != "console-role" {
		t.Fatalf("role should come from the shared config: %+v", merged)
	}
}
