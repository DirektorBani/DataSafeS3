package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// AC-AUD-16: DELETE folder without recursive returns 409 + object_count.
func TestDeleteFolder_notEmptyReturnsObjectCount(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/folder-cnt", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	// Create folder marker + one object under prefix
	req = authReq(http.MethodPut, "/api/v1/buckets/folder-cnt/folders", adminTok, []byte(`{"name":"docs"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		// createFolder may return nested shape — also try put object under prefix
		t.Logf("create folder status %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/folder-cnt/objects/docs/a.txt", adminTok, []byte("one"))
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put object %d %s", rec.Code, rec.Body.String())
	}
	req = authReq(http.MethodPut, "/api/v1/buckets/folder-cnt/objects/docs/b.txt", adminTok, []byte("two"))
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put object2 %d", rec.Code)
	}

	delBody := []byte(`{"prefix":"docs/","recursive":false}`)
	req = authReq(http.MethodDelete, "/api/v1/buckets/folder-cnt/folders", adminTok, delBody)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Error       string `json:"error"`
		ObjectCount int    `json:"object_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error != "folder not empty" {
		t.Fatalf("error %q", got.Error)
	}
	if got.ObjectCount < 2 {
		t.Fatalf("object_count=%d want >=2", got.ObjectCount)
	}

	// Recursive succeeds
	delBody = []byte(`{"prefix":"docs/","recursive":true}`)
	req = authReq(http.MethodDelete, "/api/v1/buckets/folder-cnt/folders", adminTok, delBody)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("recursive delete %d %s", rec.Code, rec.Body.String())
	}
}
