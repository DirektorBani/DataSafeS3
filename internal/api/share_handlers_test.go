package api_test

import (
	"bytes"
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

func TestShareLink_hashOnlyStorage(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/hash-share", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/hash-share/objects/secret.txt", tok, []byte("hash-test"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)

	body, _ := json.Marshal(map[string]any{"key": "secret.txt", "expires_in_sec": 3600})
	req = authReq(http.MethodPost, "/api/v1/buckets/hash-share/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create share %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Share.Token == "" {
		t.Fatal("missing token in create response")
	}

	stored, err := s.Meta().GetSharedLink(created.Share.ID)
	if err != nil {
		t.Fatalf("get stored share: %v", err)
	}
	if stored.Token != "" {
		t.Fatalf("plaintext token retained in store: %q", stored.Token)
	}
	if stored.TokenHash == "" {
		t.Fatal("expected token hash in store")
	}
	if stored.TokenHash != metadata.HashShareToken(created.Share.Token) {
		t.Fatalf("token hash mismatch stored=%q want=%q", stored.TokenHash, metadata.HashShareToken(created.Share.Token))
	}

	listReq := authReq(http.MethodGet, "/api/v1/buckets/hash-share/shares", tok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, listReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list shares %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Shares []struct {
			Token     string `json:"token"`
			TokenHash string `json:"token_hash"`
		} `json:"shares"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	for _, sh := range listed.Shares {
		if sh.Token != "" {
			t.Fatalf("list leaked plaintext token %q", sh.Token)
		}
		if sh.TokenHash != "" {
			t.Fatalf("list leaked token hash %q", sh.TokenHash)
		}
	}

	info := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, info)
	if rec.Code != http.StatusOK {
		t.Fatalf("share info via plaintext token %d %s", rec.Code, rec.Body.String())
	}
}

func TestSharedLinkDownloadAndLimit(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/share-test", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/share-test/objects/secret.txt", tok, []byte("shared-data"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)

	body, _ := json.Marshal(map[string]any{
		"key": "secret.txt", "expires_in_sec": 3600, "max_downloads": 1,
	})
	req = authReq(http.MethodPost, "/api/v1/buckets/share-test/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create share %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
		URL string `json:"url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Share.Token == "" {
		t.Fatal("missing token")
	}
	if !strings.Contains(created.URL, "/share/"+created.Share.Token) {
		t.Fatalf("share url want /share/{token} got %q", created.URL)
	}

	info := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, info)
	if rec.Code != http.StatusOK {
		t.Fatalf("share info %d %s", rec.Code, rec.Body.String())
	}
	var meta struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &meta)
	if meta.Filename != "secret.txt" {
		t.Fatalf("filename %q", meta.Filename)
	}

	dl := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, dl)
	if rec.Code != http.StatusOK {
		t.Fatalf("first download %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "shared-data" {
		t.Fatalf("body %q", got)
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, dl)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second download want 403 got %d", rec.Code)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errBody)
	if errBody.Error != "download limit reached" {
		t.Fatalf("error body %q", errBody.Error)
	}
}

func TestPublicReadBucketS3Get(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")

	body, _ := json.Marshal(map[string]string{"visibility": "public-read"})
	req := authReq(http.MethodPost, "/api/v1/buckets/public-bkt", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/public-bkt/objects/open.txt", tok, []byte("public"))
	putReq.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)

	anon := httptest.NewRequest(http.MethodGet, "/public-bkt/open.txt", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, anon)
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous get %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "public" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestTenantMembersCRUD(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	createBody, _ := json.Marshal(map[string]string{
		"username": "tmuser", "password": "pass12345", "role": auth.RoleUser, "email": "tm@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, createBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var userResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &userResp)

	tenantBody, _ := json.Marshal(map[string]string{"name": "Acme"})
	req = authReq(http.MethodPost, "/api/v1/tenants", adminTok, tenantBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var tenantResp struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tenantResp)

	addBody, _ := json.Marshal(map[string]string{"user_id": userResp.ID, "role": "member"})
	req = authReq(http.MethodPost, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members", adminTok, addBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add member %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var list struct {
		Members []map[string]any `json:"members"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(list.Members))
	}

	updBody, _ := json.Marshal(map[string]string{"role": "tenant_admin"})
	req = authReq(http.MethodPut, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members/"+userResp.ID, adminTok, updBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update role %d", rec.Code)
	}

	req = authReq(http.MethodDelete, "/api/v1/tenants/"+tenantResp.Tenant.ID+"/members/"+userResp.ID, adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove member %d", rec.Code)
	}
}

func TestSharedLinkExpired(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/exp-share", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/exp-share/objects/x.txt", tok, []byte("x"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)

	body, _ := json.Marshal(map[string]any{"key": "x.txt", "expires_in_sec": 1})
	req = authReq(http.MethodPost, "/api/v1/buckets/exp-share/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	time.Sleep(1100 * time.Millisecond)
	dl := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, dl)
	if rec.Code != http.StatusGone {
		t.Fatalf("expired share want 410 got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPrivateBucketRejectsAnonymousS3Get(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/private-bkt", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	putReq := authReq(http.MethodPut, "/api/v1/buckets/private-bkt/objects/hidden.txt", tok, []byte("secret"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, putReq)

	anon := httptest.NewRequest(http.MethodGet, "/private-bkt/hidden.txt", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, anon)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("private bucket anonymous get want 403 got %d", rec.Code)
	}
	_, _ = io.ReadAll(rec.Body)
	_ = bytes.Reader{}
}
