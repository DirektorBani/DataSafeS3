package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// AC-OL-1 / AC-OL-2: Admin settings expose and persist retention_mode (v1.2.0 Stage 1).
func TestPutBucketSettings_retentionMode(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/lock-mode", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"object_lock_enabled": true,
		"retention_days":      30,
		"retention_mode":      "COMPLIANCE",
	})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/lock-mode", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/lock-mode/settings", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		ObjectLock    bool   `json:"object_lock_enabled"`
		RetentionDays int    `json:"retention_days"`
		RetentionMode string `json:"retention_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.ObjectLock || got.RetentionDays != 30 || got.RetentionMode != "COMPLIANCE" {
		t.Fatalf("unexpected settings %+v", got)
	}

	bad, _ := json.Marshal(map[string]any{"retention_mode": "INVALID"})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/lock-mode", adminTok, bad)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode, got %d", rec.Code)
	}
}

// AC-VER-1a: Admin settings expose and persist versioning_suspended (v1.2.0 Stage 2).
func TestPutBucketSettings_versioningSuspended(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/ver-suspend", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"versioning_enabled":   true,
		"versioning_suspended": true,
	})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/ver-suspend", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/ver-suspend/settings", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings %d", rec.Code)
	}
	var got struct {
		Versioning          bool `json:"versioning_enabled"`
		VersioningSuspended bool `json:"versioning_suspended"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Versioning || !got.VersioningSuspended {
		t.Fatalf("expected enabled+suspended, got %+v", got)
	}

	off, _ := json.Marshal(map[string]any{"versioning_enabled": false})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/ver-suspend", adminTok, off)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable versioning %d", rec.Code)
	}
	req = authReq(http.MethodGet, "/api/v1/buckets/ver-suspend/settings", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Versioning || got.VersioningSuspended {
		t.Fatalf("disabling versioning should clear suspended: %+v", got)
	}
}
