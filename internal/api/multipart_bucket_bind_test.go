package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// REST multipart used to authorize the bucket in the URL and then pass the
// upload record's own bucket into the service. UploadPart compares those two
// arguments, so the check always passed and a known uploadId could be written
// through any bucket the caller can write.
func TestMultipartPartRejectsBucketMismatch(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "bucket-a", nil)
	createBucket(t, s, tok, "bucket-b", nil)

	uploadID := initMultipartUpload(t, s, tok, "bucket-b", "secret.bin")
	recB, err := s.Meta().GetBucket("bucket-b")
	if err != nil {
		t.Fatal(err)
	}
	storageB := recB.EffectiveStorageKey()

	part := []byte("cross-bucket-bytes")
	req := authReq(http.MethodPut, "/api/v1/buckets/bucket-a/multipart/"+uploadID+"/parts/1", tok, part)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-bucket part status %d body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "upload not found" {
		t.Fatalf("error %v", body["error"])
	}
	parts, err := s.Svc().Backend.ListParts(context.Background(), storageB, "secret.bin", uploadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Fatalf("part was stored on bucket-b: %+v", parts)
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/bucket-b/multipart/"+uploadID+"/parts/1", tok, part)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same-bucket part status %d body %s", rec.Code, rec.Body.String())
	}
	parts, err = s.Svc().Backend.ListParts(context.Background(), storageB, "secret.bin", uploadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].PartNumber != 1 {
		t.Fatalf("parts %+v", parts)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/bucket-a/multipart/"+uploadID+"/complete", tok, []byte(`{"parts":[{"part_number":1,"etag":"abc"}]}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-bucket complete status %d body %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodDelete, "/api/v1/buckets/bucket-a/multipart/"+uploadID, tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-bucket abort status %d body %s", rec.Code, rec.Body.String())
	}
	if _, err := s.Meta().GetMultipart(uploadID); err != nil {
		t.Fatalf("upload removed by cross-bucket abort: %v", err)
	}

	req = authReq(http.MethodDelete, "/api/v1/buckets/bucket-b/multipart/"+uploadID, tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("same-bucket abort status %d body %s", rec.Code, rec.Body.String())
	}
}

func initMultipartUpload(t *testing.T, s interface{ Handler() http.Handler }, tok, bucket, key string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"key": key})
	req := authReq(http.MethodPost, "/api/v1/buckets/"+bucket+"/multipart", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("init %s status %d body %s", bucket, rec.Code, rec.Body.String())
	}
	var resp struct {
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.UploadID == "" {
		t.Fatalf("no upload id: %s", rec.Body.String())
	}
	return resp.UploadID
}
