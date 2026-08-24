package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestInventoryJobCSVExport(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/inv-src", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/inv-src/objects/a/hello.txt", tok, []byte("hello-evidence"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	jobBody, _ := json.Marshal(map[string]string{
		"bucket": "inv-src",
		"prefix": "a/",
		"format": "csv",
	})
	req = authReq(http.MethodPost, "/api/v1/inventory/jobs", tok, jobBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create job %d %s", rec.Code, rec.Body.String())
	}
	var job map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	if job["status"] != "completed" {
		t.Fatalf("expected completed got %+v", job)
	}
	id, _ := job["id"].(string)
	if id == "" {
		t.Fatal("missing id")
	}

	req = authReq(http.MethodGet, "/api/v1/inventory/jobs/"+id+"/download", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download %d %s", rec.Code, rec.Body.String())
	}
	csv := rec.Body.String()
	if !strings.Contains(csv, "object_lock_enabled") || !strings.Contains(csv, "a/hello.txt") {
		t.Fatalf("unexpected csv: %s", csv)
	}
}

func TestActivityExportCSVAndAuditRow(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/activity/export?format=csv&period=all", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("content-type %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "action") {
		t.Fatalf("csv header missing: %s", rec.Body.String())
	}

	listReq := authReq(http.MethodGet, "/api/v1/activity?action=activity_exported&period=all&limit=5", tok, nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list %d", listRec.Code)
	}
	if !strings.Contains(listRec.Body.String(), "activity_exported") {
		t.Fatalf("expected activity_exported event: %s", listRec.Body.String())
	}
}

func TestObjectDeleteBlockedWritesActivity(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/lock-ev", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	settings, _ := json.Marshal(map[string]any{
		"object_lock_enabled": true,
		"retention_days":      30,
		"retention_mode":      "COMPLIANCE",
	})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/lock-ev", tok, settings)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/lock-ev/objects/protected.txt", tok, []byte("secret"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodDelete, "/api/v1/buckets/lock-ev/objects/protected.txt", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d %s", rec.Code, rec.Body.String())
	}

	listReq := authReq(http.MethodGet, "/api/v1/activity?action=object_delete_blocked&period=all&limit=10", tok, nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "object_delete_blocked") {
		t.Fatalf("expected delete blocked activity: %s", listRec.Body.String())
	}
}

func TestActivityRetentionPurge(t *testing.T) {
	s := testServer(t)
	old := time.Now().UTC().Add(-120 * 24 * time.Hour)
	_ = s.Meta().AppendActivity(metadata.ActivityRecord{
		User: "alice", Action: metadata.ActionLogin, ResourceType: "session", ResourceName: "alice", Timestamp: old,
	})
	_ = s.Meta().AppendActivity(metadata.ActivityRecord{
		User: "bob", Action: metadata.ActionLogin, ResourceType: "session", ResourceName: "bob", Timestamp: time.Now().UTC(),
	})
	n, err := s.Meta().PurgeActivityBefore(time.Now().UTC().Add(-90 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected purge of old row, got %d", n)
	}
	res, err := s.Meta().ListActivity(metadata.ActivityFilter{Period: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Events {
		if e.User == "alice" {
			t.Fatal("old alice event should be purged")
		}
	}
}

func TestActivityExportRejectsBadFormat(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodGet, "/api/v1/activity/export?format=parquet", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestInventoryRejectsCronSchedule(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/inv-cron", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}
	jobBody, _ := json.Marshal(map[string]string{
		"bucket": "inv-cron", "format": "csv", "schedule": "cron",
	})
	req = authReq(http.MethodPost, "/api/v1/inventory/jobs", tok, jobBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestS3DeleteBlockedWritesActivity(t *testing.T) {
	s := testServer(t)
	creds := auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	sign := func(req *http.Request) {
		t.Helper()
		if req.Host == "" {
			req.Host = req.URL.Host
		}
		if err := auth.SignRequest(req, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
			t.Fatal(err)
		}
	}

	bucket := "s3-lock-ev"
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket, nil)
	sign(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("create bucket %d", resp.StatusCode)
	}

	tok := loginToken(t, s, "admin", "admin")
	settings, _ := json.Marshal(map[string]any{
		"object_lock_enabled": true,
		"retention_days":      30,
		"retention_mode":      "COMPLIANCE",
	})
	adminReq := authReq(http.MethodPut, "/api/v1/settings/buckets/"+bucket, tok, settings)
	adminRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("settings %d %s", adminRec.Code, adminRec.Body.String())
	}

	body := bytes.NewReader([]byte("locked"))
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/prot.txt", body)
	sign(req)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("put %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/"+bucket+"/prot.txt", nil)
	sign(req)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 got %d", resp.StatusCode)
	}

	listReq := authReq(http.MethodGet, "/api/v1/activity?action=object_delete_blocked&period=all&limit=20", tok, nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "object_delete_blocked") {
		t.Fatalf("expected S3 delete blocked activity: %s", listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "prot.txt") {
		t.Fatalf("expected object key in activity: %s", listRec.Body.String())
	}
}

func TestInventoryJobDestBucket(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	for _, name := range []string{"inv-a", "inv-dest"} {
		req := authReq(http.MethodPost, "/api/v1/buckets/"+name, tok, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("create %s %d %s", name, rec.Code, rec.Body.String())
		}
	}
	req := authReq(http.MethodPut, "/api/v1/buckets/inv-a/objects/x.bin", tok, bytes.Repeat([]byte("x"), 8))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	jobBody, _ := json.Marshal(map[string]string{
		"bucket":      "inv-a",
		"format":      "csv",
		"dest_bucket": "inv-dest",
		"dest_key":    "reports/inv-a.csv",
	})
	req = authReq(http.MethodPost, "/api/v1/inventory/jobs", tok, jobBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("job %d %s", rec.Code, rec.Body.String())
	}
	var job map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	if job["status"] != "completed" {
		t.Fatalf("%+v", job)
	}

	rc, _, err := s.Svc().GetObject(context.Background(), "inv-dest", "reports/inv-a.csv", "")
	if err != nil {
		t.Fatalf("dest get: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if !strings.Contains(string(data), "x.bin") {
		t.Fatalf("dest csv missing object: %s", data)
	}
}
