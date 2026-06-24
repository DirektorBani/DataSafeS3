package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestSearchNumericBucketName(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/3", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket 3: %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/search?q=3", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"results"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total < 1 {
		t.Fatalf("expected search hit for bucket 3, got %+v", resp)
	}
	found := false
	for _, r := range resp.Results {
		if r.Type == "bucket" && r.Name == "3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bucket 3 not in results: %+v", resp.Results)
	}
}

func TestTrashRetentionValidation(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	for _, days := range []int{4000, -1} {
		body, _ := json.Marshal(map[string]any{
			"soft_delete_enabled":  true,
			"trash_retention_days": days,
		})
		req := authReq(http.MethodPut, "/api/v1/settings/system", tok, body)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("days=%d expected 400 got %d %s", days, rec.Code, rec.Body.String())
		}
	}

	body, _ := json.Marshal(map[string]any{
		"soft_delete_enabled":  true,
		"trash_retention_days": 45,
	})
	req := authReq(http.MethodPut, "/api/v1/settings/system", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid retention: %d %s", rec.Code, rec.Body.String())
	}
}

func TestWebhookCreateWithHeadersAndEvents(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]any{
		"name":    "alerts",
		"url":     "https://example.com/hook",
		"events":  []string{"ObjectCreated", "BucketDeleted", "UserCreated"},
		"headers": map[string]string{"X-Custom": "test"},
		"enabled": true,
	})
	req := authReq(http.MethodPost, "/api/v1/webhooks", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create webhook %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Webhook struct {
			Events  []string          `json:"events"`
			Headers map[string]string `json:"headers"`
			Enabled bool              `json:"enabled"`
		} `json:"webhook"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Webhook.Events) != 3 {
		t.Fatalf("events: %+v", resp.Webhook.Events)
	}
	if resp.Webhook.Headers["X-Custom"] != "test" {
		t.Fatalf("headers: %+v", resp.Webhook.Headers)
	}
	if !resp.Webhook.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestMultipartQuotaEnforcement(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	updateBody := []byte(`{"max_size_bytes":20}`)
	req := authReq(http.MethodPut, "/api/v1/users/admin-bootstrap", tok, updateBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set quota %d", rec.Code)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/mpq", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	initBody, _ := json.Marshal(map[string]string{"key": "big.bin", "content_type": "application/octet-stream"})
	req = authReq(http.MethodPost, "/api/v1/buckets/mpq/multipart", tok, initBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("init multipart %d %s", rec.Code, rec.Body.String())
	}
	var initResp struct {
		UploadID string `json:"upload_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &initResp)
	if initResp.UploadID == "" {
		t.Fatalf("no upload_id: %s", rec.Body.String())
	}

	part := bytes.Repeat([]byte("x"), 30)
	req = authReq(http.MethodPut, "/api/v1/buckets/mpq/multipart/"+initResp.UploadID+"/parts/1", tok, part)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload part %d %s", rec.Code, rec.Body.String())
	}

	var partResp struct {
		ETag string `json:"etag"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &partResp)

	completeBody, _ := json.Marshal(map[string]any{
		"parts": []map[string]any{{"part_number": 1, "etag": partResp.ETag}},
	})
	req = authReq(http.MethodPost, "/api/v1/buckets/mpq/multipart/"+initResp.UploadID+"/complete", tok, completeBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected quota exceeded on multipart complete, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayHealthEndpoint(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/gateway/health", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("gateway health %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if _, ok := resp["connections_total"]; !ok {
		t.Fatalf("missing connections_total: %+v", resp)
	}
}

func TestSoftDeleteObjectWithOrphanTrashBucketDir(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]any{
		"soft_delete_enabled":  true,
		"trash_retention_days": 7,
	})
	req := authReq(http.MethodPut, "/api/v1/settings/system", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable soft delete: %d %s", rec.Code, rec.Body.String())
	}

	// Trash bucket dir on disk without metadata (e.g. metadata reset while data dir kept).
	if err := s.Svc().Backend.CreateBucket(metadata.TrashBucketName); err != nil {
		t.Fatalf("seed trash dir: %v", err)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/23", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket 23: %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/23/objects/gateway-replication-test.txt", tok, []byte("payload"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload object: %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodDelete, "/api/v1/buckets/23/objects/gateway-replication-test.txt", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete object with soft delete: %d %s", rec.Code, rec.Body.String())
	}
	var delResp struct {
		Trashed bool `json:"trashed"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &delResp)
	if !delResp.Trashed {
		t.Fatalf("expected trashed=true, got %s", rec.Body.String())
	}

	if _, err := s.Meta().GetBucket(metadata.TrashBucketName); err != nil {
		t.Fatalf("trash bucket not registered in metadata: %v", err)
	}
	if !s.Svc().Backend.BucketExists(metadata.TrashBucketName) {
		t.Fatal("trash bucket dir missing on backend")
	}
}
