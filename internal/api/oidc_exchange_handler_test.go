package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleOIDCExchange_singleUse(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServer(Config{DataDir: dir, JWTSecret: "test-jwt"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	code, err := s.oidcExchange.Issue("session-jwt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"exchange_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/exchange", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first exchange: %d %s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/exchange", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("second exchange: %d", rec2.Code)
	}
}
