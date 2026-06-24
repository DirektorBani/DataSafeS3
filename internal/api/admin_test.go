package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
)

func testServer(t *testing.T) *api.Server {
	t.Helper()
	s := freshTestServer(t)
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func freshTestServer(t *testing.T) *api.Server {
	t.Helper()
	if _, ok := os.LookupEnv("STORAGE_AUTO_HOME_BUCKET"); !ok {
		t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	}
	dir := t.TempDir()
	s, err := api.NewServer(api.Config{
		DataDir:       dir,
		AdminUser:     "admin",
		AdminPassword: "admin",
		JWTSecret:     "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func loginToken(t *testing.T, s *api.Server, user, pass string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp["token"]
}

func authReq(method, path, token string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestRBACUsersAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/users", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list users %d", rec.Code)
	}

	// Create operator user
	createBody, _ := json.Marshal(map[string]string{
		"username": "op", "password": "op123", "role": "operator", "email": "op@test.com",
	})
	req = authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create operator %d %s", rec.Code, rec.Body.String())
	}

	opTok := loginToken(t, s, "op", "op123")
	req = authReq(http.MethodGet, "/api/v1/users", opTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator list users want 403 got %d", rec.Code)
	}
}

func TestActivityAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createBody, _ := json.Marshal(map[string]string{
		"username": "actuser", "password": "usr123", "role": auth.RoleUser, "email": "a@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "actuser", "usr123")
	req = authReq(http.MethodGet, "/api/v1/activity?limit=10", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user activity want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/activity?limit=10", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin activity %d %s", rec.Code, rec.Body.String())
	}
}

func TestActivityEndpoint(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/activity?limit=10", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("activity %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Events []map[string]any `json:"events"`
		Total  int              `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total < 1 {
		t.Fatalf("expected login event, got %d", resp.Total)
	}
}

func TestUsageEndpoint(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/testb", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodGet, "/api/v1/usage", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage %d %s", rec.Code, rec.Body.String())
	}
}

func TestPolicyAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/polbucket", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	createBody, _ := json.Marshal(map[string]string{
		"username": "poluser", "password": "usr123", "role": auth.RoleUser, "email": "p@test.com",
	})
	req = authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "poluser", "usr123")
	req = authReq(http.MethodGet, "/api/v1/buckets/polbucket/policy", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user policy want 403 got %d", rec.Code)
	}
}

func TestAccessKeyOwnership(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createBody, _ := json.Marshal(map[string]string{
		"username": "keyuser", "password": "usr123", "role": auth.RoleUser, "email": "k@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "keyuser", "usr123")
	keyBody, _ := json.Marshal(map[string]string{"label": "mine"})
	req = authReq(http.MethodPost, "/api/v1/keys", userTok, keyBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	req = authReq(http.MethodGet, "/api/v1/keys", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Keys []map[string]any `json:"keys"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Keys) != 1 {
		t.Fatalf("user should see 1 key, got %d", len(list.Keys))
	}

	req = authReq(http.MethodGet, "/api/v1/keys", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Keys) < 2 {
		t.Fatalf("admin should see bootstrap + user keys, got %d", len(list.Keys))
	}

	req = authReq(http.MethodDelete, "/api/v1/keys/"+created["access_key"], adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete key %d", rec.Code)
	}
}

func TestScheduledDelete(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/sdel", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/sdel/objects/foo.txt", tok, []byte("hello"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodDelete, "/api/v1/buckets/sdel/objects/foo.txt?schedule=1d", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("schedule delete %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/sdel/objects", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var objs struct {
		Objects []map[string]any `json:"objects"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &objs)
	if len(objs.Objects) != 1 {
		t.Fatalf("object should still exist, got %d", len(objs.Objects))
	}
	if objs.Objects[0]["scheduled_delete_at"] == nil {
		t.Fatal("expected scheduled_delete_at on object")
	}
}

func TestSettingsAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createBody, _ := json.Marshal(map[string]string{
		"username": "usr", "password": "usr123", "role": auth.RoleUser, "email": "u@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "usr", "usr123")
	req = authReq(http.MethodGet, "/api/v1/settings/buckets", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user settings want 403 got %d", rec.Code)
	}
}

func TestUserBucketIsolation(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createBody, _ := json.Marshal(map[string]string{
		"username": "usr2", "password": "usr123", "role": auth.RoleUser, "email": "u2@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "usr2", "usr123")
	req = authReq(http.MethodPost, "/api/v1/buckets/mybucket", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var resp struct {
		Buckets []map[string]any `json:"buckets"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Buckets) != 1 {
		t.Fatalf("user should see 1 bucket, got %d", len(resp.Buckets))
	}
}
