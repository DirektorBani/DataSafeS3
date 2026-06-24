package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAndFavorites(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	// Create bucket and object for search
	req := authReq(http.MethodPost, "/api/v1/buckets/search-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	body := []byte("hello search")
	req = authReq(http.MethodPut, "/api/v1/buckets/search-test/objects/report.txt", tok, body)
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/search?q=report", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search %d %s", rec.Code, rec.Body.String())
	}
	var searchResp struct {
		Results []struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		} `json:"results"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &searchResp)
	if searchResp.Total < 1 {
		t.Fatalf("expected search hits, got %v", searchResp)
	}

	// Favorites
	favBody, _ := json.Marshal(map[string]string{"type": "bucket", "bucket": "search-test"})
	req = authReq(http.MethodPost, "/api/v1/favorites", tok, favBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create favorite %d %s", rec.Code, rec.Body.String())
	}
	var favResp struct {
		Favorite struct {
			ID string `json:"id"`
		} `json:"favorite"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &favResp)

	req = authReq(http.MethodGet, "/api/v1/favorites", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list favorites %d", rec.Code)
	}

	req = authReq(http.MethodDelete, "/api/v1/favorites/"+favResp.Favorite.ID, tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete favorite %d", rec.Code)
	}
}

func TestObjectTagsAndMeta(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/tags-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	tagBody, _ := json.Marshal(map[string]any{"tags": map[string]string{"env": "prod"}})
	req = authReq(http.MethodPut, "/api/v1/buckets/tags-test/tags", tok, tagBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put bucket tags %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPut, "/api/v1/buckets/tags-test/objects/file.txt", tok, []byte("data"))
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	objTagBody, _ := json.Marshal(map[string]any{"tags": map[string]string{"type": "doc"}})
	req = authReq(http.MethodPut, "/api/v1/buckets/tags-test/object-tags?key=file.txt", tok, objTagBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put object tags %d %s", rec.Code, rec.Body.String())
	}

	metaBody, _ := json.Marshal(map[string]any{
		"metadata":     map[string]string{"x-amz-meta-author": "test"},
		"content_type": "application/json",
	})
	req = authReq(http.MethodPut, "/api/v1/buckets/tags-test/object-meta?key=file.txt", tok, metaBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put object meta %d %s", rec.Code, rec.Body.String())
	}
}

func TestListObjectsPagination(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/page-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		req = authReq(http.MethodPut, "/api/v1/buckets/page-test/objects/"+name, tok, []byte("x"))
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/page-test/objects?max_keys=2", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var page1 struct {
		Objects    []struct{ Key string } `json:"objects"`
		Truncated  bool                   `json:"truncated"`
		NextMarker string                 `json:"next_marker"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page1)
	if !page1.Truncated || page1.NextMarker == "" || len(page1.Objects) != 2 {
		t.Fatalf("page1: truncated=%v marker=%q count=%d", page1.Truncated, page1.NextMarker, len(page1.Objects))
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/page-test/objects?max_keys=2&start_after="+page1.NextMarker, tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var page2 struct {
		Objects []struct{ Key string } `json:"objects"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page2)
	if len(page2.Objects) != 1 {
		t.Fatalf("page2 count %d", len(page2.Objects))
	}
}

func TestObjectRename(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/rename-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	req = authReq(http.MethodPut, "/api/v1/buckets/rename-test/objects/old.txt", tok, []byte("rename me"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	renameBody, _ := json.Marshal(map[string]string{
		"action": "rename", "key": "old.txt", "dest_key": "new.txt",
	})
	req = authReq(http.MethodPost, "/api/v1/buckets/rename-test/object-actions", tok, renameBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/rename-test/objects?prefix=new", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !bytes.Contains(rec.Body.Bytes(), []byte("new.txt")) {
		t.Fatalf("expected new.txt in listing: %s", rec.Body.String())
	}
}
