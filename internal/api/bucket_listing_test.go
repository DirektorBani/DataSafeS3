package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListObjectsDelimitedShowsRootFiles(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/root-files-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket: %d %s", rec.Code, rec.Body.String())
	}

	// Many nested objects to fill the first page if pagination is wrong.
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("nested/deep/object-%02d.bin", i)
		req = authReq(http.MethodPut, "/api/v1/buckets/root-files-test/objects/"+key, tok, []byte("x"))
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("put nested %s: %d", key, rec.Code)
		}
	}
	req = authReq(http.MethodPut, "/api/v1/buckets/root-files-test/objects/readme.txt", tok, []byte("hello root"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put root file: %d", rec.Code)
	}

	req = authReq(http.MethodGet, "/api/v1/buckets/root-files-test/objects?delimiter=/&max_keys=10", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Folders []string `json:"folders"`
		Objects []struct {
			Key string `json:"key"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	foundRoot := false
	for _, o := range resp.Objects {
		if o.Key == "readme.txt" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("root file readme.txt missing from first page: folders=%v objects=%v", resp.Folders, resp.Objects)
	}
	foundNestedFolder := false
	for _, f := range resp.Folders {
		if f == "nested/" {
			foundNestedFolder = true
		}
	}
	if !foundNestedFolder {
		t.Fatalf("expected nested/ folder in listing, got folders=%v", resp.Folders)
	}
}

func TestChangePasswordLocalUser(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]any{
		"username": "localpw", "password": "oldpass123", "role": "user", "email": "lp@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body.String())
	}

	userTok := loginToken(t, s, "localpw", "oldpass123")
	changeBody, _ := json.Marshal(map[string]string{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	})
	req = authReq(http.MethodPost, "/api/v1/me/password", userTok, changeBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
	}
	if loginToken(t, s, "localpw", "newpass456") == "" {
		t.Fatal("login with new password failed")
	}
}
