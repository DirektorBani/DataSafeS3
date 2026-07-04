package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminMFASetupRequired_blocksBucketsUntilEnrolled(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	cfg, _ := s.Meta().GetSystemConfig()
	cfg.MFA.RequireAdminMFA = true
	_ = s.Meta().PutSystemConfig(cfg)

	admin, _ := s.Meta().GetUserByUsername("admin")
	admin.MFAEnabled = false
	admin.WebAuthnCredentials = ""
	_ = s.Meta().UpdateUser(admin)

	req := authReq(http.MethodGet, "/api/v1/buckets", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("buckets want 403 before MFA enroll, got %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["mfa_setup_required"] != true {
		t.Fatalf("expected mfa_setup_required, got %v", body)
	}

	req = authReq(http.MethodPost, "/api/v1/mfa/enroll", adminTok, []byte(`{}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mfa enroll want 200 got %d", rec.Code)
	}
}
