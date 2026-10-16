package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnrichAPIErrorKeepsLegacyText(t *testing.T) {
	body := map[string]any{"error": "invalid credentials", "mfa_token": "secret-token"}
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "req-1")
	out := enrichAPIError(rec, http.StatusUnauthorized, body)
	if body["code"] != nil {
		t.Fatal("enrichAPIError mutated the caller map")
	}
	if out["error"] != "invalid credentials" {
		t.Fatalf("legacy error %v", out["error"])
	}
	if out["code"] != codeAuthInvalidCredentials {
		t.Fatalf("code %v", out["code"])
	}
	if out["message"] != "Invalid credentials" {
		t.Fatalf("message %v", out["message"])
	}
	if out["request_id"] != "req-1" {
		t.Fatalf("request_id %v", out["request_id"])
	}
	details, _ := out["details"].(map[string]any)
	if details["mfa_token"] != "secret-token" {
		t.Fatalf("details %v", details)
	}
	if out["mfa_token"] != "secret-token" {
		t.Fatal("extra field was dropped from the top level")
	}
}

func TestErrorCodePasswordChangeRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	out := enrichAPIError(rec, http.StatusForbidden, map[string]any{"error": "password_change_required"})
	if out["code"] != codeSetupPasswordChange {
		t.Fatalf("code %v", out["code"])
	}
	if out["error"] != "password_change_required" {
		t.Fatalf("legacy %v", out["error"])
	}
}

func TestAssignRequestIDRejectsControlCharacters(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	req.Header.Set("X-Request-Id", "bad\nid")
	assignRequestID(rec, req)
	got := rec.Header().Get("X-Request-Id")
	if got == "" || got == "bad\nid" {
		t.Fatalf("request id %q", got)
	}
}
