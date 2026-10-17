package main

import (
	"net/http"
	"testing"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
		stepUp bool
	}{
		{401, `{"error":"unauthorized"}`, "auth.unauthorized", false},
		{401, `{"error":"session expired"}`, "auth.session_expired", false},
		{401, `{"error":"step_up_required","action":"delete_bucket"}`, "auth.step_up_required", true},
		{401, `{"error":"mfa_token_invalid"}`, "auth.mfa_token_invalid", true},
		{401, `{"error":"invalid code"}`, "auth.invalid_code", true},
		{401, `{"error":"nope","code":"auth.session_expired"}`, "auth.session_expired", false},
		{403, `{"mfa_setup_required":true}`, "auth.mfa_setup_required", false},
		{403, `{"error":"mfa_required","action":"bucket_delete"}`, "auth.mfa_required", false},
		{403, `{"error":"forbidden"}`, "access.forbidden", false},
		{404, `{"error":"upload not found"}`, "multipart.upload_not_found", false},
		{404, `{"error":"not found"}`, "resource.not_found", false},
		{400, `{"error":"invalid json"}`, "request.invalid", false},
		{409, `{"error":"bucket exists"}`, "resource.conflict", false},
		{429, `{"error":"rate limit exceeded"}`, "rate_limited", false},
		{500, `{"error":"boom"}`, "internal", false},
		{401, `not-json`, "auth.unauthorized", false},
	}
	for _, tc := range cases {
		got := classifyError(tc.status, []byte(tc.body), nil)
		if got.Code != tc.code || got.StepUp != tc.stepUp {
			t.Fatalf("status %d body %s → %+v", tc.status, tc.body, got)
		}
		if reauthCode(got) != (tc.status == http.StatusUnauthorized && !tc.stepUp) {
			t.Fatalf("reauth for %+v", got)
		}
	}
	header := make(http.Header)
	header.Set("Retry-After", "15")
	got := classifyError(429, []byte(`{"error":"rate limit exceeded"}`), header)
	if got.RetryAfter != 15 {
		t.Fatalf("retry after %d", got.RetryAfter)
	}
	setup := classifyError(http.StatusForbidden, []byte(`{"error":"mfa_required"}`), nil)
	if !setup.MFASetup || setup.StepUp {
		t.Fatalf("mfa required should be a setup banner: %+v", setup)
	}
	replay := classifyError(http.StatusUnauthorized, []byte(`{"error":"mfa_token_invalid"}`), nil)
	if !replay.StepUp || reauthCode(replay) {
		t.Fatalf("replayed step-up token should open the dialog again: %+v", replay)
	}
}
