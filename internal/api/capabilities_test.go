package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilitiesPublicAndAuthenticated(t *testing.T) {
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "https://vault.example.test:8200")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	s := testServer(t)
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.OIDC.Enabled = true
	cfg.OIDC.Issuer = "https://idp.example.test/realms/desk"
	cfg.OIDC.ClientID = "desktop"
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}

	anon := httptest.NewRecorder()
	s.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	if anon.Code != http.StatusOK {
		t.Fatalf("anon %d %s", anon.Code, anon.Body.String())
	}
	if cc := anon.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Fatalf("anon cache %q", cc)
	}
	var public map[string]any
	if err := json.Unmarshal(anon.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"features", "clients", "deprecations"} {
		if _, ok := public[forbidden]; ok {
			t.Fatalf("anonymous body included %s: %s", forbidden, anon.Body.String())
		}
	}
	if public["api"] != "v1" || public["spec_version"] != "1.6.1" {
		t.Fatalf("public identity %v", public)
	}
	if public["server_version"] != "1.6.0" {
		t.Fatalf("unstamped server_version %v", public["server_version"])
	}
	assertNoSecretLeak(t, anon.Body.String())

	token := loginToken(t, s, "admin", "admin")
	authed := httptest.NewRecorder()
	s.Handler().ServeHTTP(authed, authReq(http.MethodGet, "/api/v1/capabilities", token, nil))
	if authed.Code != http.StatusOK {
		t.Fatalf("auth %d %s", authed.Code, authed.Body.String())
	}
	if cc := authed.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("auth cache %q", cc)
	}
	var full map[string]any
	if err := json.Unmarshal(authed.Body.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	features, _ := full["features"].(map[string]any)
	for _, key := range []string{"auth.password", "auth.totp", "auth.oidc", "auth.step_up", "objects.multipart", "sharing.links", "sharing.policy", "sharing.revoke_all", "encryption.kms", "errors.codes"} {
		if features[key] != float64(1) {
			t.Fatalf("feature %s = %v", key, features[key])
		}
	}
	for _, absent := range []string{"auth.refresh", "objects.list_parts", "objects.range", "server.secrets_stdin", "server.schema_version"} {
		if _, ok := features[absent]; ok {
			t.Fatalf("feature %s should be omitted", absent)
		}
	}
	clients, _ := full["clients"].(map[string]any)
	desktop, _ := clients["desktop"].(map[string]any)
	if desktop["min_supported"] != "1.0.0" || desktop["max_tested"] != nil || desktop["min_required"] != nil {
		t.Fatalf("desktop compat %v", desktop)
	}
	deps, ok := full["deprecations"].([]any)
	if !ok || len(deps) != 0 {
		t.Fatalf("deprecations %v", full["deprecations"])
	}
	assertNoSecretLeak(t, authed.Body.String())

	bad := httptest.NewRecorder()
	s.Handler().ServeHTTP(bad, authReq(http.MethodGet, "/api/v1/capabilities", "not-a-token", nil))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad token %d %s", bad.Code, bad.Body.String())
	}
	var errBody map[string]any
	if err := json.Unmarshal(bad.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody["error"] != "unauthorized" || errBody["code"] != "auth.unauthorized" {
		t.Fatalf("401 body %v", errBody)
	}
}

func assertNoSecretLeak(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{"vault.example.test", "idp.example.test", "client_secret", "datasafesecret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("capabilities leaked %q in %s", secret, body)
		}
	}
}

func TestCapabilitiesLocalLoginOnlyByDefault(t *testing.T) {
	s := testServer(t)
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OIDC.Enabled {
		t.Fatal("fresh config should not enable OIDC")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	methods, _ := body["login_methods"].([]any)
	if len(methods) != 1 || methods[0] != "password" {
		t.Fatalf("login_methods %v", methods)
	}
}
