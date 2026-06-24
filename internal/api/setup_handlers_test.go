package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestSetupStatusPublic(t *testing.T) {
	s := freshTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["needs_setup"] != true {
		t.Fatalf("expected needs_setup true, got %v", resp["needs_setup"])
	}
	if resp["initial_setup_completed"] != false {
		t.Fatalf("expected initial_setup_completed false, got %v", resp["initial_setup_completed"])
	}
}

func TestAdminFirstLoginSetsFlag(t *testing.T) {
	s := freshTestServer(t)
	_ = loginToken(t, s, "admin", "admin")

	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AdminFirstLoginCompleted {
		t.Fatal("expected admin_first_login_completed after login")
	}
}

func TestSetupGuardBlocksAdmin(t *testing.T) {
	s := freshTestServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/buckets", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 setup guard, got %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "setup_required" {
		t.Fatalf("expected setup_required, got %q", resp["error"])
	}
}

func TestSetupS3TestInvalidEndpoint(t *testing.T) {
	s := freshTestServer(t)
	tok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]any{
		"endpoint":          "http://127.0.0.1:9",
		"access_key_id":     "k",
		"secret_access_key": "s",
		"bucket":            "test",
		"region":            "us-east-1",
		"use_ssl":           false,
	})
	req := authReq(http.MethodPost, "/api/v1/setup/s3/test", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("test endpoint status %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["ok"] == true {
		t.Fatal("expected connection test to fail against closed port")
	}
}

func TestSetupS3SaveSetsFlag(t *testing.T) {
	s := freshTestServer(t)
	tok := loginToken(t, s, "admin", "admin")

	cfg, _ := s.Meta().GetSystemConfig()
	cfg.InitialSetupCompleted = true
	cfg.ExternalS3 = metadata.ExternalS3Config{
		Endpoint:        "http://127.0.0.1:9",
		AccessKeyID:     "k",
		SecretAccessKey: "s",
		Bucket:          "b",
		Region:          "us-east-1",
		UseSSL:          false,
	}
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var status map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &status)
	if status["needs_setup"] != false {
		t.Fatal("expected needs_setup false after manual complete")
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected buckets access after setup, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSetupCompleteSkip(t *testing.T) {
	s := freshTestServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/setup/complete", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 before password change, got %d %s", rec.Code, rec.Body.String())
	}

	changeBody, _ := json.Marshal(map[string]string{
		"current_password": "admin",
		"new_password":   "Admin123!",
	})
	req = authReq(http.MethodPost, "/api/v1/me/password", tok, changeBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPost, "/api/v1/setup/complete", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete setup: %d %s", rec.Code, rec.Body.String())
	}

	cfg, _ := s.Meta().GetSystemConfig()
	if !cfg.InitialSetupCompleted {
		t.Fatal("expected initial_setup_completed after skip")
	}
	if !cfg.AdminPasswordChanged {
		t.Fatal("expected admin_password_changed after password change")
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected buckets access after skip setup, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSetupStatusNeedsPasswordChange(t *testing.T) {
	s := freshTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["needs_password_change"] != true {
		t.Fatalf("expected needs_password_change true on fresh install, got %v", resp["needs_password_change"])
	}
}

func TestSetupS3SaveRequiresValidConnection(t *testing.T) {
	s := freshTestServer(t)
	tok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]any{
		"endpoint":          "http://127.0.0.1:9",
		"access_key_id":     "k",
		"secret_access_key": "s",
		"bucket":            "test",
		"region":            "us-east-1",
		"use_ssl":           false,
	})
	req := authReq(http.MethodPost, "/api/v1/setup/s3/save", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403 on failed save, got %d %s", rec.Code, rec.Body.String())
	}
	cfg, _ := s.Meta().GetSystemConfig()
	if cfg.InitialSetupCompleted {
		t.Fatal("initial_setup_completed must stay false when save fails")
	}
}
