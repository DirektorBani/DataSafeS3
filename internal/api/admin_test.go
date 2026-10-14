package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/buildinfo"
	"github.com/DirektorBani/datasafe/internal/observability"
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
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	dir := t.TempDir()
	s, err := api.NewServer(api.Config{
		DataDir:       dir,
		AdminUser:     "admin",
		AdminPassword: "admin",
		JWTSecret:     "test",
		SSEMasterKey:  "test-sse-master-key",
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

func TestEventDeliveryAdminOnly(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	createBody, _ := json.Marshal(map[string]string{
		"username": "evuser", "password": "usr123", "role": auth.RoleUser, "email": "e@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	userTok := loginToken(t, s, "evuser", "usr123")
	req = authReq(http.MethodGet, "/api/v1/events/delivery", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user delivery want 403 got %d", rec.Code)
	}
	req = authReq(http.MethodPost, "/api/v1/events/test", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user test want 403 got %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/events/delivery", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin delivery %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		QueueDepth int `json:"queue_depth"`
		Sinks      []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sinks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.QueueDepth != 0 {
		t.Fatalf("queue %d", body.QueueDepth)
	}
	sawKafka := false
	for _, sk := range body.Sinks {
		if sk.Name == "kafka" {
			sawKafka = true
			if sk.Status != "disabled" {
				t.Fatalf("kafka status %s", sk.Status)
			}
		}
	}
	if !sawKafka {
		t.Fatalf("sinks %+v", body.Sinks)
	}

	before := deliverySinkCounts(t, rec.Body.Bytes(), "webhook")
	observability.AddEventsDelivered("webhook")
	observability.AddEventsDelivered("webhook")
	observability.AddEventsFailed("webhook", "timeout")
	observability.AddEventsFailed("webhook", "http_5xx")
	req = authReq(http.MethodGet, "/api/v1/events/delivery", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin delivery counts %d %s", rec.Code, rec.Body.String())
	}
	after := deliverySinkCounts(t, rec.Body.Bytes(), "webhook")
	if after.delivered != before.delivered+2 || after.failed != before.failed+2 {
		t.Fatalf("webhook counters before %+v after %+v", before, after)
	}

	req = authReq(http.MethodPost, "/api/v1/events/test", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("test with no sinks want 409 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestEventDeliveryUnreachableBrokerDoesNotBlock(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "192.0.2.1:9092")
	t.Setenv("STORAGE_KAFKA_SASL_MECHANISM", "")
	t.Setenv("STORAGE_NATS_URL", "")
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
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.StartBackground(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		cancel()
		ts.Close()
	})
	adminTok := loginToken(t, s, "admin", "admin")
	client := &http.Client{Timeout: 2 * time.Second}

	deadline := time.Now().Add(8 * time.Second)
	var status, last string
	for {
		start := time.Now()
		body := eventDeliveryBody(t, client, ts.URL, adminTok)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("GET /api/v1/events/delivery took %s", elapsed)
		}
		status, last = kafkaDelivery(t, body)
		if status != "degraded" {
			t.Fatalf("kafka status %s body %s", status, body)
		}
		if last == "" {
			t.Fatalf("degraded with empty last_error: %s", body)
		}
		if brokerError(last) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last_error %q after probe window", last)
		}
		time.Sleep(30 * time.Millisecond)
	}

	start := time.Now()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/events/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminTok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("POST /api/v1/events/test took %s", elapsed)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("test event %d", resp.StatusCode)
	}
	start = time.Now()
	_ = eventDeliveryBody(t, client, ts.URL, adminTok)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("GET during produce took %s", elapsed)
	}
}

type sinkCountBody struct {
	delivered uint64
	failed    uint64
}

func deliverySinkCounts(t *testing.T, raw []byte, name string) sinkCountBody {
	t.Helper()
	var body struct {
		Sinks []struct {
			Name      string `json:"name"`
			Delivered uint64 `json:"delivered"`
			Failed    uint64 `json:"failed"`
		} `json:"sinks"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, sk := range body.Sinks {
		if sk.Name == name {
			return sinkCountBody{delivered: sk.Delivered, failed: sk.Failed}
		}
	}
	t.Fatalf("sink %s missing in %s", name, raw)
	return sinkCountBody{}
}

func TestHealthReportsVersionAndCommit(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/healthz", "/api/v1/health"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var body struct {
			Status  string `json:"status"`
			Version string `json:"version"`
			Commit  string `json:"commit"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "ok" || body.Version != buildinfo.Version || body.Commit != buildinfo.Commit {
			t.Fatalf("%s %+v", path, body)
		}
		if body.Version == "" || body.Commit == "" {
			t.Fatalf("%s empty version or commit", path)
		}
	}
}

func eventDeliveryBody(t *testing.T, client *http.Client, base, token string) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/events/delivery", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delivery %d %s", resp.StatusCode, body)
	}
	return body
}

func kafkaDelivery(t *testing.T, body []byte) (string, string) {
	t.Helper()
	var parsed struct {
		Sinks []struct {
			Name      string `json:"name"`
			Status    string `json:"status"`
			LastError string `json:"last_error"`
		} `json:"sinks"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, sk := range parsed.Sinks {
		if sk.Name == "kafka" {
			return sk.Status, sk.LastError
		}
	}
	t.Fatalf("no kafka sink in %s", body)
	return "", ""
}

func brokerError(last string) bool {
	return strings.Contains(last, "timeout") || strings.Contains(last, "refused") || strings.Contains(last, "dial") || strings.Contains(last, "deadline")
}
