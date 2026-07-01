package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestHooksTest_blocksPrivateIP(t *testing.T) {
	t.Setenv("STORAGE_DEV", "false")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]string{"url": "http://169.254.169.254/"})
	req := authReq(http.MethodPost, "/api/v1/hooks/test", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "outbound url not allowed") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestPutLoggingConfig_blocksLoopbackLoki(t *testing.T) {
	t.Setenv("STORAGE_DEV", "false")
	t.Setenv("STORAGE_OUTBOUND_HTTP_ALLOW", "")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	cfg, _ := s.Meta().GetSystemConfig()
	cfg.Logging.Loki = metadata.LogSinkEndpoint{Enabled: true, Address: "http://127.0.0.1:3100"}
	raw, _ := json.Marshal(cfg)
	req := authReq(http.MethodPut, "/api/v1/settings/system", tok, raw)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestPutLoggingConfig_allowsLoopbackWhenOutboundHTTPAllow(t *testing.T) {
	t.Setenv("STORAGE_DEV", "false")
	t.Setenv("STORAGE_OUTBOUND_HTTP_ALLOW", "true")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	cfg, _ := s.Meta().GetSystemConfig()
	cfg.Logging.Loki = metadata.LogSinkEndpoint{Enabled: true, Address: "http://127.0.0.1:3100"}
	raw, _ := json.Marshal(cfg)
	req := authReq(http.MethodPut, "/api/v1/settings/system", tok, raw)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOIDCROPC_disabled(t *testing.T) {
	t.Setenv("STORAGE_OIDC_ROPC_ENABLED", "false")
	s := testServer(t)
	cfg, _ := s.Meta().GetSystemConfig()
	cfg.OIDC.Enabled = true
	_ = s.Meta().PutSystemConfig(cfg)
	body, _ := json.Marshal(map[string]string{"username": "u", "password": "p"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/password-login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	t.Setenv("STORAGE_RATE_LIMIT_LOGIN", "2")
	t.Setenv("STORAGE_RATE_LIMIT_WINDOW", "1m")
	dir := t.TempDir()
	s, err := api.NewServer(api.Config{DataDir: dir, JWTSecret: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
		req.RemoteAddr = "203.0.113.9:12345"
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestSecurityStatus_listsWeakSecrets(t *testing.T) {
	t.Setenv("STORAGE_DEV", "false")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodGet, "/api/v1/settings/security-status", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		WeakSecrets     []string `json:"weak_secrets"`
		FieldEncryption struct {
			Enabled bool `json:"enabled"`
		} `json:"field_encryption"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.WeakSecrets) == 0 {
		t.Fatal("expected weak secrets in non-dev mode")
	}
	if resp.FieldEncryption.Enabled {
		t.Fatal("expected field_encryption.enabled false by default")
	}
}

func TestCreateWebhook_blocksSSRF(t *testing.T) {
	t.Setenv("STORAGE_DEV", "false")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]any{
		"name": "bad", "url": "http://127.0.0.1/hook", "events": []string{"test"},
	})
	req := authReq(http.MethodPost, "/api/v1/webhooks", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestPutSystemConfig_rejectsLDAPWithoutTLS(t *testing.T) {
	t.Setenv("STORAGE_LDAP_REQUIRE_TLS", "true")
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	cfg, _ := s.Meta().GetSystemConfig()
	cfg.LDAP = metadata.LDAPConfig{
		Enabled: true,
		URL:     "ldap://ldap.example.com:389",
	}
	raw, _ := json.Marshal(cfg)
	req := authReq(http.MethodPut, "/api/v1/settings/system", tok, raw)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "tls") {
		t.Fatalf("expected TLS error, body %s", rec.Body.String())
	}
}

func TestCORS_allowlistOrigin(t *testing.T) {
	t.Setenv("STORAGE_CORS_ALLOWED_ORIGINS", "http://localhost:8080")
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("allowed origin: got %q", got)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req2.Header.Set("Origin", "http://evil.com")
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got == "http://evil.com" || got == "*" {
		t.Fatalf("evil origin should not be reflected: %q", got)
	}
}
