package api_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp/totp"
	"gopkg.in/yaml.v3"
)

// TestDesktopOpenAPIConformance hits every x-clients desktop operation once
// for a success status and once for each documented error status.
func TestDesktopOpenAPIConformance(t *testing.T) {
	s := testServer(t)
	token := loginToken(t, s, "admin", "admin")
	guest, err := s.TokenForTest(auth.TokenInfo{Username: "guest", UserID: "guest-id", Role: "guest"})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("unused-pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Meta().PutUser(metadata.UserRecord{
		ID: "oidc-desk", Username: "oidcdesk", PasswordHash: hash,
		Role: metadata.RoleUser, Status: metadata.StatusActive, AuthSource: "oidc",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	oidcTok, err := s.TokenForTest(auth.TokenInfo{Username: "oidcdesk", UserID: "oidc-desk", Role: auth.RoleUser})
	if err != nil {
		t.Fatal(err)
	}

	hit := map[string]map[string]bool{}
	note := func(op, status string) {
		if hit[op] == nil {
			hit[op] = map[string]bool{}
		}
		hit[op][status] = true
	}
	call := func(op, method, path, body, who string, want int) *httptest.ResponseRecorder {
		t.Helper()
		var tok string
		switch who {
		case "admin":
			tok = token
		case "guest":
			tok = guest
		case "oidc":
			tok = oidcTok
		case "bad":
			tok = "not-a-token"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Request-Id", "conformance-"+op)
		if body != "" && !strings.Contains(path, "/objects/") {
			req.Header.Set("Content-Type", "application/json")
		}
		if strings.Contains(path, "/objects/") && method == http.MethodPut && who == "admin" && !strings.Contains(path, "/multipart/") {
			req.Header.Set("Content-Type", "text/plain")
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s %s: want %d got %d body %s", op, method, path, want, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Request-Id") == "" {
			t.Fatalf("%s missing X-Request-Id", op)
		}
		if want >= 400 && want != http.StatusNoContent {
			assertErrorContract(t, op, rec.Body.Bytes())
		}
		if hit[op] == nil {
			hit[op] = map[string]bool{}
		}
		hit[op][itoa(want)] = true
		return rec
	}

	call("healthCheck", http.MethodGet, "/api/v1/health", "", "none", 200)
	call("getHealthz", http.MethodGet, "/healthz", "", "none", 200)
	call("getLoginOptions", http.MethodGet, "/api/v1/auth/login-options", "", "none", 200)
	call("getSetupStatus", http.MethodGet, "/api/v1/setup/status", "", "none", 200)
	call("getOIDCConfig", http.MethodGet, "/api/v1/auth/oidc/config", "", "none", 200)
	call("getCapabilities", http.MethodGet, "/api/v1/capabilities", "", "none", 200)
	call("getCapabilities", http.MethodGet, "/api/v1/capabilities", "", "bad", 401)

	call("adminLogin", http.MethodPost, "/api/v1/admin/login", `{"username":"admin","password":"admin","client":"desktop"}`, "none", 200)
	call("adminLogin", http.MethodPost, "/api/v1/admin/login", `{`, "none", 400)
	call("adminLogin", http.MethodPost, "/api/v1/admin/login", `{"username":"admin","password":"wrong"}`, "none", 401)

	call("getMe", http.MethodGet, "/api/v1/me", "", "admin", 200)
	call("getMe", http.MethodGet, "/api/v1/me", "", "none", 401)
	call("changePassword", http.MethodPost, "/api/v1/me/password", `{"current_password":"admin","new_password":"admin-2"}`, "admin", 200)
	token = loginToken(t, s, "admin", "admin-2")
	call("changePassword", http.MethodPost, "/api/v1/me/password", `{`, "admin", 400)
	call("changePassword", http.MethodPost, "/api/v1/me/password", `{"current_password":"admin-2","new_password":"x"}`, "none", 401)
	call("changePassword", http.MethodPost, "/api/v1/me/password", `{"current_password":"x","new_password":"y"}`, "oidc", 403)
	call("logout", http.MethodPost, "/api/v1/admin/logout", "", "admin", 204)
	call("logout", http.MethodPost, "/api/v1/admin/logout", "", "none", 401)

	call("listBuckets", http.MethodGet, "/api/v1/buckets", "", "admin", 200)
	call("listBuckets", http.MethodGet, "/api/v1/buckets", "", "none", 401)
	call("listBuckets", http.MethodGet, "/api/v1/buckets", "", "guest", 403)
	call("createBucket", http.MethodPost, "/api/v1/buckets/desk", `{}`, "admin", 201)
	call("createBucket", http.MethodPost, "/api/v1/buckets/nope", `{}`, "none", 401)
	call("createBucket", http.MethodPost, "/api/v1/buckets/nope", `{}`, "guest", 403)
	call("createBucket", http.MethodPost, "/api/v1/buckets/gone", `{}`, "admin", 201)

	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/hello.txt", "hello", "admin", 200)
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/hello.txt", "hello", "none", 401)
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/hello.txt", "hello", "guest", 403)
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/sched.txt", "sched", "admin", 200)
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/perm.txt", "perm", "admin", 200)
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/copy-src.txt", "copy", "admin", 200)

	call("listObjects", http.MethodGet, "/api/v1/buckets/desk/objects", "", "admin", 200)
	call("listObjects", http.MethodGet, "/api/v1/buckets/missing/objects", "", "admin", 404)
	call("listObjects", http.MethodGet, "/api/v1/buckets/desk/objects", "", "none", 401)
	call("listObjects", http.MethodGet, "/api/v1/buckets/desk/objects", "", "guest", 403)
	call("listObjectVersions", http.MethodGet, "/api/v1/buckets/desk/versions", "", "admin", 200)
	call("listObjectVersions", http.MethodGet, "/api/v1/buckets/missing/versions", "", "admin", 404)
	call("listObjectVersions", http.MethodGet, "/api/v1/buckets/desk/versions", "", "none", 401)
	call("listObjectVersions", http.MethodGet, "/api/v1/buckets/desk/versions", "", "guest", 403)
	call("getBucketSettings", http.MethodGet, "/api/v1/buckets/desk/settings", "", "admin", 200)
	call("getBucketSettings", http.MethodGet, "/api/v1/buckets/missing/settings", "", "admin", 404)
	call("getBucketSettings", http.MethodGet, "/api/v1/buckets/desk/settings", "", "none", 401)
	call("getBucketSettings", http.MethodGet, "/api/v1/buckets/desk/settings", "", "guest", 403)
	call("downloadObject", http.MethodGet, "/api/v1/buckets/desk/objects/hello.txt", "", "admin", 200)
	call("downloadObject", http.MethodGet, "/api/v1/buckets/desk/objects/missing.txt", "", "admin", 404)
	call("downloadObject", http.MethodGet, "/api/v1/buckets/desk/objects/hello.txt", "", "none", 401)
	call("downloadObject", http.MethodGet, "/api/v1/buckets/desk/objects/hello.txt", "", "guest", 403)
	call("getObjectMeta", http.MethodGet, "/api/v1/buckets/desk/object-meta?key=hello.txt", "", "admin", 200)
	call("getObjectMeta", http.MethodGet, "/api/v1/buckets/desk/object-meta?key=missing.txt", "", "admin", 404)
	call("getObjectMeta", http.MethodGet, "/api/v1/buckets/desk/object-meta?key=hello.txt", "", "none", 401)
	call("getObjectMeta", http.MethodGet, "/api/v1/buckets/desk/object-meta?key=hello.txt", "", "guest", 403)
	call("putObjectMeta", http.MethodPut, "/api/v1/buckets/desk/object-meta?key=hello.txt", `{"content_type":"text/plain"}`, "admin", 200)
	call("putObjectMeta", http.MethodPut, "/api/v1/buckets/missing/object-meta?key=hello.txt", `{"content_type":"text/plain"}`, "admin", 404)
	call("putObjectMeta", http.MethodPut, "/api/v1/buckets/desk/object-meta?key=hello.txt", `{"content_type":"text/plain"}`, "none", 401)
	call("putObjectMeta", http.MethodPut, "/api/v1/buckets/desk/object-meta?key=hello.txt", `{"content_type":"text/plain"}`, "guest", 403)
	call("getObjectTags", http.MethodGet, "/api/v1/buckets/desk/object-tags?key=hello.txt", "", "admin", 200)
	call("getObjectTags", http.MethodGet, "/api/v1/buckets/missing/object-tags?key=hello.txt", "", "admin", 404)
	call("getObjectTags", http.MethodGet, "/api/v1/buckets/desk/object-tags?key=hello.txt", "", "none", 401)
	call("getObjectTags", http.MethodGet, "/api/v1/buckets/desk/object-tags?key=hello.txt", "", "guest", 403)
	call("putObjectTags", http.MethodPut, "/api/v1/buckets/desk/object-tags?key=hello.txt", `{"tags":{"k":"v"}}`, "admin", 200)
	call("putObjectTags", http.MethodPut, "/api/v1/buckets/missing/object-tags?key=hello.txt", `{"tags":{"k":"v"}}`, "admin", 404)
	call("putObjectTags", http.MethodPut, "/api/v1/buckets/desk/object-tags?key=hello.txt", `{"tags":{"k":"v"}}`, "none", 401)
	call("putObjectTags", http.MethodPut, "/api/v1/buckets/desk/object-tags?key=hello.txt", `{"tags":{"k":"v"}}`, "guest", 403)
	call("getObjectRetention", http.MethodGet, "/api/v1/buckets/desk/object-retention?key=hello.txt", "", "admin", 200)
	call("getObjectRetention", http.MethodGet, "/api/v1/buckets/missing/object-retention?key=hello.txt", "", "admin", 404)
	call("getObjectRetention", http.MethodGet, "/api/v1/buckets/desk/object-retention?key=hello.txt", "", "none", 401)
	call("getObjectRetention", http.MethodGet, "/api/v1/buckets/desk/object-retention?key=hello.txt", "", "guest", 403)
	call("getBucketEncryption", http.MethodGet, "/api/v1/buckets/desk/encryption", "", "admin", 200)
	call("getBucketEncryption", http.MethodGet, "/api/v1/buckets/missing/encryption", "", "admin", 404)
	call("getBucketEncryption", http.MethodGet, "/api/v1/buckets/desk/encryption", "", "none", 401)
	call("getBucketEncryption", http.MethodGet, "/api/v1/buckets/desk/encryption", "", "guest", 403)
	call("getBucketSharing", http.MethodGet, "/api/v1/buckets/desk/sharing", "", "admin", 200)
	call("getBucketSharing", http.MethodGet, "/api/v1/buckets/missing/sharing", "", "admin", 404)
	call("getBucketSharing", http.MethodGet, "/api/v1/buckets/desk/sharing", "", "none", 401)
	call("createFolder", http.MethodPost, "/api/v1/buckets/desk/folders", `{"name":"docs"}`, "admin", 201)
	call("createFolder", http.MethodPost, "/api/v1/buckets/desk/folders", `{"name":"docs"}`, "none", 401)
	call("createFolder", http.MethodPost, "/api/v1/buckets/desk/folders", `{"name":"docs"}`, "guest", 403)
	call("deleteFolder", http.MethodDelete, "/api/v1/buckets/desk/folders", `{"prefix":"docs"}`, "admin", 200)
	call("deleteFolder", http.MethodDelete, "/api/v1/buckets/missing/folders", `{"prefix":"docs"}`, "admin", 404)
	call("deleteFolder", http.MethodDelete, "/api/v1/buckets/desk/folders", `{"prefix":"docs"}`, "none", 401)
	call("deleteFolder", http.MethodDelete, "/api/v1/buckets/desk/folders", `{"prefix":"docs"}`, "guest", 403)
	call("bulkDeleteObjects", http.MethodPost, "/api/v1/buckets/desk/bulk-delete", `{"keys":["no-such.txt"]}`, "admin", 200)
	call("bulkDeleteObjects", http.MethodPost, "/api/v1/buckets/desk/bulk-delete", `{`, "admin", 400)
	call("bulkDeleteObjects", http.MethodPost, "/api/v1/buckets/desk/bulk-delete", `{"keys":["no-such.txt"]}`, "none", 401)
	call("bulkDeleteObjects", http.MethodPost, "/api/v1/buckets/desk/bulk-delete", `{"keys":["no-such.txt"]}`, "guest", 403)
	call("objectActions", http.MethodPost, "/api/v1/buckets/desk/object-actions", `{"action":"copy","key":"copy-src.txt","dest_key":"copy-dst.txt"}`, "admin", 200)
	call("objectActions", http.MethodPost, "/api/v1/buckets/desk/object-actions", `{"action":"copy","key":"copy-src.txt","dest_key":"copy-dst.txt"}`, "none", 401)
	call("objectActions", http.MethodPost, "/api/v1/buckets/desk/object-actions", `{"action":"copy","key":"copy-src.txt","dest_key":"copy-dst.txt"}`, "guest", 403)
	call("presignURL", http.MethodPost, "/api/v1/presign", `{"bucket":"desk","key":"hello.txt","method":"GET"}`, "admin", 200)
	call("presignURL", http.MethodPost, "/api/v1/presign", `{`, "admin", 400)
	call("presignURL", http.MethodPost, "/api/v1/presign", `{"bucket":"desk","key":"hello.txt"}`, "none", 401)
	call("presignURL", http.MethodPost, "/api/v1/presign", `{"bucket":"desk","key":"hello.txt"}`, "guest", 403)
	call("listTrash", http.MethodGet, "/api/v1/trash", "", "admin", 200)
	call("listTrash", http.MethodGet, "/api/v1/trash", "", "none", 401)
	call("listTrash", http.MethodGet, "/api/v1/trash", "", "guest", 403)

	init := call("initiateMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart", `{"key":"multi.bin"}`, "admin", 200)
	uploadID := jsonString(t, init.Body.Bytes(), "upload_id")
	part := call("uploadMultipartPart", http.MethodPut, "/api/v1/buckets/desk/multipart/"+uploadID+"/parts/1", "part-bytes", "admin", 200)
	etag := jsonString(t, part.Body.Bytes(), "etag")
	completeBody, _ := json.Marshal(map[string]any{"parts": []map[string]any{{"part_number": 1, "etag": etag}}})
	call("uploadMultipartPart", http.MethodPut, "/api/v1/buckets/missing/multipart/"+uploadID+"/parts/1", "part-bytes", "admin", 404)
	call("uploadMultipartPart", http.MethodPut, "/api/v1/buckets/desk/multipart/"+uploadID+"/parts/1", "part-bytes", "none", 401)
	call("uploadMultipartPart", http.MethodPut, "/api/v1/buckets/desk/multipart/"+uploadID+"/parts/1", "part-bytes", "guest", 403)
	call("completeMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart/"+uploadID+"/complete", string(completeBody), "admin", 200)
	call("completeMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart/"+uploadID+"/complete", `{"parts":[{"part_number":1,"etag":"x"}]}`, "none", 401)
	call("completeMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart/"+uploadID+"/complete", `{"parts":[{"part_number":1,"etag":"x"}]}`, "guest", 403)
	abort := call("initiateMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart", `{"key":"abort.bin"}`, "admin", 200)
	abortID := jsonString(t, abort.Body.Bytes(), "upload_id")
	call("abortMultipart", http.MethodDelete, "/api/v1/buckets/desk/multipart/"+abortID, "", "admin", 204)
	call("abortMultipart", http.MethodDelete, "/api/v1/buckets/missing/multipart/"+abortID, "", "admin", 404)
	call("abortMultipart", http.MethodDelete, "/api/v1/buckets/desk/multipart/"+abortID, "", "none", 401)
	call("abortMultipart", http.MethodDelete, "/api/v1/buckets/desk/multipart/"+abortID, "", "guest", 403)
	call("initiateMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart", `{"key":"multi.bin"}`, "none", 401)
	call("initiateMultipart", http.MethodPost, "/api/v1/buckets/desk/multipart", `{"key":"multi.bin"}`, "guest", 403)

	share := call("createShare", http.MethodPost, "/api/v1/buckets/desk/shares", `{"key":"hello.txt","expires_in_sec":3600}`, "admin", 201)
	shareID := nestedString(t, share.Body.Bytes(), "share", "id")
	call("createShare", http.MethodPost, "/api/v1/buckets/desk/shares", `{`, "admin", 400)
	call("createShare", http.MethodPost, "/api/v1/buckets/missing/shares", `{"key":"hello.txt"}`, "admin", 404)
	call("createShare", http.MethodPost, "/api/v1/buckets/desk/shares", `{"key":"hello.txt"}`, "none", 401)
	call("createShare", http.MethodPost, "/api/v1/buckets/desk/shares", `{"key":"hello.txt"}`, "guest", 403)
	call("listShares", http.MethodGet, "/api/v1/buckets/desk/shares", "", "admin", 200)
	call("listShares", http.MethodGet, "/api/v1/buckets/missing/shares", "", "admin", 404)
	call("listShares", http.MethodGet, "/api/v1/buckets/desk/shares", "", "none", 401)
	call("listShares", http.MethodGet, "/api/v1/buckets/desk/shares", "", "guest", 403)
	call("revokeShare", http.MethodDelete, "/api/v1/shares/"+shareID, "", "admin", 204)
	call("revokeShare", http.MethodDelete, "/api/v1/shares/missing-share", "", "admin", 404)
	call("revokeShare", http.MethodDelete, "/api/v1/shares/"+shareID, "", "none", 401)
	call("revokeShare", http.MethodDelete, "/api/v1/shares/"+shareID, "", "guest", 403)
	call("revokeAllShares", http.MethodPost, "/api/v1/buckets/desk/shares/revoke-all", "", "admin", 200)
	call("revokeAllShares", http.MethodPost, "/api/v1/buckets/missing/shares/revoke-all", "", "admin", 404)
	call("revokeAllShares", http.MethodPost, "/api/v1/buckets/desk/shares/revoke-all", "", "none", 401)
	call("revokeAllShares", http.MethodPost, "/api/v1/buckets/desk/shares/revoke-all", "", "guest", 403)

	call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/sched.txt?schedule=1d", "", "admin", 200)
	call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/perm.txt?permanent=true", "", "admin", 204)
	call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/missing.txt", "", "admin", 404)
	call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/hello.txt", "", "none", 401)
	call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/hello.txt", "", "guest", 403)

	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SoftDeleteEnabled = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	call("uploadObject", http.MethodPut, "/api/v1/buckets/desk/objects/trash.txt", "trash", "admin", 200)
	trashed := call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/trash.txt", "", "admin", 200)
	trashID := jsonString(t, trashed.Body.Bytes(), "trash_id")
	call("restoreTrash", http.MethodPost, "/api/v1/trash/"+trashID+"/restore", "", "admin", 200)
	call("restoreTrash", http.MethodPost, "/api/v1/trash/"+trashID+"/restore", "", "none", 401)
	call("restoreTrash", http.MethodPost, "/api/v1/trash/"+trashID+"/restore", "", "guest", 403)
	trashed = call("deleteObject", http.MethodDelete, "/api/v1/buckets/desk/objects/trash.txt", "", "admin", 200)
	trashID = jsonString(t, trashed.Body.Bytes(), "trash_id")
	call("purgeTrash", http.MethodDelete, "/api/v1/trash/"+trashID, "", "guest", 403)
	call("purgeTrash", http.MethodDelete, "/api/v1/trash/"+trashID, "", "none", 401)
	call("purgeTrash", http.MethodDelete, "/api/v1/trash/missing-trash", "", "admin", 404)
	call("purgeTrash", http.MethodDelete, "/api/v1/trash/"+trashID, "", "admin", 204)

	call("deleteBucket", http.MethodDelete, "/api/v1/buckets/gone", "", "admin", 204)
	call("deleteBucket", http.MethodDelete, "/api/v1/buckets/missing", "", "admin", 404)
	call("deleteBucket", http.MethodDelete, "/api/v1/buckets/desk", "", "none", 401)
	call("deleteBucket", http.MethodDelete, "/api/v1/buckets/desk", "", "guest", 403)
	call("completeSetup", http.MethodPost, "/api/v1/setup/complete", "", "admin", 200)
	call("completeSetup", http.MethodPost, "/api/v1/setup/complete", "", "none", 401)

	fresh := freshTestServer(t)
	freshTok := loginToken(t, fresh, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/setup/complete", freshTok, []byte(`{}`))
	req.Header.Set("X-Request-Id", "conformance-completeSetup")
	rec := httptest.NewRecorder()
	fresh.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("completeSetup fresh %d %s", rec.Code, rec.Body.String())
	}
	assertErrorContract(t, "completeSetup", rec.Body.Bytes())
	var setupErr map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &setupErr)
	if setupErr["error"] != "password_change_required" || setupErr["code"] != "setup.password_change_required" {
		t.Fatalf("setup 403 %v", setupErr)
	}
	hit["completeSetup"]["403"] = true

	oidcLogin400 := call("oidcLogin", http.MethodGet, "/api/v1/auth/oidc/login", "", "none", 400)
	_ = oidcLogin400
	call("oidcCallback", http.MethodGet, "/api/v1/auth/oidc/callback", "", "none", 400)
	call("oidcExchange", http.MethodPost, "/api/v1/auth/oidc/exchange", `{}`, "none", 400)
	call("oidcExchange", http.MethodPost, "/api/v1/auth/oidc/exchange", `{"exchange_code":"nope"}`, "none", 401)
	call("oidcPasswordLogin", http.MethodPost, "/api/v1/auth/oidc/password-login", `{"username":"a","password":"b"}`, "none", 400)

	t.Run("oidc", func(t *testing.T) {
		issuer := startFakeOIDC(t)
		srv := testServer(t)
		cfg, err := srv.Meta().GetSystemConfig()
		if err != nil {
			t.Fatal(err)
		}
		cfg.OIDC.Enabled = true
		cfg.OIDC.Issuer = issuer
		cfg.OIDC.InternalIssuer = issuer
		cfg.OIDC.ClientID = "desktop"
		cfg.OIDC.ClientSecret = "secret"
		cfg.OIDC.RedirectURL = "http://127.0.0.1/callback"
		if err := srv.Meta().PutSystemConfig(cfg); err != nil {
			t.Fatal(err)
		}
		login := httptest.NewRecorder()
		srv.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil))
		if login.Code != http.StatusFound || login.Header().Get("Location") == "" {
			t.Fatalf("oidc login %d %s", login.Code, login.Body.String())
		}
		note("oidcLogin", "302")
		cb := httptest.NewRecorder()
		srv.Handler().ServeHTTP(cb, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=s", nil))
		if cb.Code != http.StatusFound {
			t.Fatalf("oidc callback %d %s", cb.Code, cb.Body.String())
		}
		loc, err := url.Parse(cb.Header().Get("Location"))
		if err != nil || loc.Query().Get("exchange_code") == "" {
			t.Fatalf("callback location %q", cb.Header().Get("Location"))
		}
		note("oidcCallback", "302")
		ex := httptest.NewRecorder()
		raw := `{"exchange_code":"` + loc.Query().Get("exchange_code") + `"}`
		srv.Handler().ServeHTTP(ex, httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/exchange", strings.NewReader(raw)))
		if ex.Code != http.StatusOK {
			t.Fatalf("exchange %d %s", ex.Code, ex.Body.String())
		}
		note("oidcExchange", "200")
		pw := httptest.NewRecorder()
		srv.Handler().ServeHTTP(pw, httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/password-login", strings.NewReader(`{"username":"a","password":"b"}`)))
		if pw.Code != http.StatusOK {
			t.Fatalf("ropc %d %s", pw.Code, pw.Body.String())
		}
		note("oidcPasswordLogin", "200")
	})
	t.Run("ropc-disabled", func(t *testing.T) {
		t.Setenv("STORAGE_OIDC_ROPC_ENABLED", "false")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/password-login", strings.NewReader(`{"username":"a","password":"b"}`)))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("ropc %d %s", rec.Code, rec.Body.String())
		}
		assertErrorContract(t, "oidcPasswordLogin", rec.Body.Bytes())
		note("oidcPasswordLogin", "403")
	})

	t.Run("mfa", func(t *testing.T) {
		srv := testServer(t)
		tok := loginToken(t, srv, "admin", "admin")
		secret, recovery := enrollTOTP(t, srv.Handler(), tok)
		if len(recovery) == 0 {
			t.Fatal("enroll returned no recovery codes")
		}
		login := callOn(t, srv, http.MethodPost, "/api/v1/admin/login", `{"username":"admin","password":"admin"}`, "")
		if login.Code != http.StatusOK {
			t.Fatalf("mfa challenge %d %s", login.Code, login.Body.String())
		}
		mfaToken := jsonString(t, login.Body.Bytes(), "mfa_token")
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		ok := callOn(t, srv, http.MethodPost, "/api/v1/mfa/login", `{"mfa_token":"`+mfaToken+`","code":"`+code+`"}`, "")
		if ok.Code != http.StatusOK {
			t.Fatalf("mfa login %d %s", ok.Code, ok.Body.String())
		}
		note("mfaLogin", "200")
		aliasCode, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		alias := callOn(t, srv, http.MethodPost, "/api/v1/auth/login/mfa", `{"mfa_token":"`+mfaToken+`","code":"`+aliasCode+`"}`, "")
		if alias.Code != http.StatusOK && alias.Code != http.StatusUnauthorized {
			t.Fatalf("mfa alias %d %s", alias.Code, alias.Body.String())
		}
		if alias.Code != http.StatusOK {
			// The same TOTP step can already have been consumed. A second code in the next window still has to succeed.
			next, err := totp.GenerateCode(secret, time.Now().Add(30*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			alias = callOn(t, srv, http.MethodPost, "/api/v1/auth/login/mfa", `{"mfa_token":"`+mfaToken+`","code":"`+next+`"}`, "")
		}
		if alias.Code != http.StatusOK {
			t.Fatalf("mfa alias %d %s", alias.Code, alias.Body.String())
		}
		note("mfaLoginAlias", "200")
		bad := callOn(t, srv, http.MethodPost, "/api/v1/mfa/login", `{`, "")
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("mfa 400 %d %s", bad.Code, bad.Body.String())
		}
		assertErrorContract(t, "mfaLogin", bad.Body.Bytes())
		note("mfaLogin", "400")
		unauth := callOn(t, srv, http.MethodPost, "/api/v1/mfa/login", `{"mfa_token":"nope","code":"000000"}`, "")
		if unauth.Code != http.StatusUnauthorized {
			t.Fatalf("mfa 401 %d %s", unauth.Code, unauth.Body.String())
		}
		assertErrorContract(t, "mfaLogin", unauth.Body.Bytes())
		note("mfaLogin", "401")
		bad = callOn(t, srv, http.MethodPost, "/api/v1/auth/login/mfa", `{`, "")
		assertErrorContract(t, "mfaLoginAlias", bad.Body.Bytes())
		note("mfaLoginAlias", "400")
		unauth = callOn(t, srv, http.MethodPost, "/api/v1/auth/login/mfa", `{"mfa_token":"nope","code":"000000"}`, "")
		assertErrorContract(t, "mfaLoginAlias", unauth.Body.Bytes())
		note("mfaLoginAlias", "401")

		step := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up", `{"action":"bucket_delete","target":"desk","code":"`+recovery[0]+`"}`, tok)
		if step.Code != http.StatusOK {
			t.Fatalf("step-up %d %s", step.Code, step.Body.String())
		}
		note("stepUp", "200")
		unknown := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up", `{"action":"nope"}`, tok)
		if unknown.Code != http.StatusBadRequest {
			t.Fatalf("step-up 400 %d %s", unknown.Code, unknown.Body.String())
		}
		assertErrorContract(t, "stepUp", unknown.Body.Bytes())
		note("stepUp", "400")
		noTok := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up", `{"action":"bucket_delete","code":"000000"}`, "")
		if noTok.Code != http.StatusUnauthorized {
			t.Fatalf("step-up 401 %d %s", noTok.Code, noTok.Body.String())
		}
		assertErrorContract(t, "stepUp", noTok.Body.Bytes())
		note("stepUp", "401")
	})

	t.Run("webauthn", func(t *testing.T) {
		srv := testServer(t)
		key := newSoftKey(t)
		user, err := srv.Meta().GetUser("admin-bootstrap")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal([]webauthn.Credential{key.cred})
		if err != nil {
			t.Fatal(err)
		}
		user.WebAuthnCredentials = string(raw)
		if err := srv.Meta().UpdateUser(user); err != nil {
			t.Fatal(err)
		}
		login := callOn(t, srv, http.MethodPost, "/api/v1/admin/login", `{"username":"admin","password":"admin"}`, "")
		mfaToken := jsonString(t, login.Body.Bytes(), "mfa_token")
		bad := callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/begin", `{`, "")
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("webauthn begin 400 %d %s", bad.Code, bad.Body.String())
		}
		assertErrorContract(t, "webauthnLoginBegin", bad.Body.Bytes())
		note("webauthnLoginBegin", "400")
		unauth := callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/begin", `{"mfa_token":"nope"}`, "")
		if unauth.Code != http.StatusUnauthorized {
			t.Fatalf("webauthn begin 401 %d %s", unauth.Code, unauth.Body.String())
		}
		assertErrorContract(t, "webauthnLoginBegin", unauth.Body.Bytes())
		note("webauthnLoginBegin", "401")
		begin := callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/begin", `{"mfa_token":"`+mfaToken+`"}`, "")
		if begin.Code != http.StatusOK {
			t.Fatalf("webauthn begin %d %s", begin.Code, begin.Body.String())
		}
		note("webauthnLoginBegin", "200")
		assertion := key.assertion(t, challengeOf(t, begin.Body.Bytes()), "http://example.com", "example.com")
		finish := callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/finish?mfa_token="+url.QueryEscape(mfaToken), string(assertion), "")
		if finish.Code != http.StatusOK {
			t.Fatalf("webauthn finish %d %s", finish.Code, finish.Body.String())
		}
		note("webauthnLoginFinish", "200")
		bad = callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/finish", "", "")
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("webauthn finish 400 %d %s", bad.Code, bad.Body.String())
		}
		assertErrorContract(t, "webauthnLoginFinish", bad.Body.Bytes())
		note("webauthnLoginFinish", "400")
		unauth = callOn(t, srv, http.MethodPost, "/api/v1/auth/mfa/webauthn/finish?mfa_token=nope", string(assertion), "")
		if unauth.Code != http.StatusUnauthorized {
			t.Fatalf("webauthn finish 401 %d %s", unauth.Code, unauth.Body.String())
		}
		assertErrorContract(t, "webauthnLoginFinish", unauth.Body.Bytes())
		note("webauthnLoginFinish", "401")

		sess, err := srv.TokenForTest(auth.TokenInfo{Username: "admin", UserID: "admin-bootstrap", Role: auth.RoleAdministrator})
		if err != nil {
			t.Fatal(err)
		}
		unknown := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/begin", `{"action":"nope"}`, sess)
		if unknown.Code != http.StatusBadRequest {
			t.Fatalf("step-up webauthn 400 %d %s", unknown.Code, unknown.Body.String())
		}
		assertErrorContract(t, "stepUpWebauthnBegin", unknown.Body.Bytes())
		note("stepUpWebauthnBegin", "400")
		noTok := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/begin", `{"action":"bucket_delete"}`, "")
		if noTok.Code != http.StatusUnauthorized {
			t.Fatalf("step-up webauthn 401 %d %s", noTok.Code, noTok.Body.String())
		}
		assertErrorContract(t, "stepUpWebauthnBegin", noTok.Body.Bytes())
		note("stepUpWebauthnBegin", "401")
		begin = callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/begin", `{"action":"bucket_delete","target":"desk"}`, sess)
		if begin.Code != http.StatusOK {
			t.Fatalf("step-up webauthn begin %d %s", begin.Code, begin.Body.String())
		}
		note("stepUpWebauthnBegin", "200")
		assertion = key.assertion(t, challengeOf(t, begin.Body.Bytes()), "http://example.com", "example.com")
		finish = callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/finish", string(assertion), sess)
		if finish.Code != http.StatusOK {
			t.Fatalf("step-up webauthn finish %d %s", finish.Code, finish.Body.String())
		}
		note("stepUpWebauthnFinish", "200")
		expired := callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/finish", string(assertion), sess)
		if expired.Code != http.StatusBadRequest {
			t.Fatalf("step-up webauthn finish 400 %d %s", expired.Code, expired.Body.String())
		}
		assertErrorContract(t, "stepUpWebauthnFinish", expired.Body.Bytes())
		note("stepUpWebauthnFinish", "400")
		noTok = callOn(t, srv, http.MethodPost, "/api/v1/auth/step-up/webauthn/finish", string(assertion), "")
		if noTok.Code != http.StatusUnauthorized {
			t.Fatalf("step-up webauthn finish 401 %d %s", noTok.Code, noTok.Body.String())
		}
		assertErrorContract(t, "stepUpWebauthnFinish", noTok.Body.Bytes())
		note("stepUpWebauthnFinish", "401")
	})

	t.Run("rate-limit", func(t *testing.T) {
		for _, op := range []struct{ id, path string }{
			{"adminLogin", "/api/v1/admin/login"},
			{"mfaLogin", "/api/v1/mfa/login"},
			{"mfaLoginAlias", "/api/v1/auth/login/mfa"},
		} {
			t.Run(op.id, func(t *testing.T) {
				t.Setenv("STORAGE_RATE_LIMIT_LOGIN", "1")
				srv := testServer(t)
				first := callOn(t, srv, http.MethodPost, op.path, `{"username":"admin","password":"wrong"}`, "")
				if first.Code == http.StatusTooManyRequests {
					t.Fatalf("%s first call was already limited", op.id)
				}
				second := callOn(t, srv, http.MethodPost, op.path, `{"username":"admin","password":"wrong"}`, "")
				if second.Code != http.StatusTooManyRequests {
					t.Fatalf("%s 429 %d %s", op.id, second.Code, second.Body.String())
				}
				if second.Header().Get("Retry-After") == "" {
					t.Fatalf("%s missing Retry-After", op.id)
				}
				assertErrorContract(t, op.id, second.Body.Bytes())
				var body map[string]any
				_ = json.Unmarshal(second.Body.Bytes(), &body)
				if body["code"] != "rate_limited" {
					t.Fatalf("%s code %v", op.id, body["code"])
				}
				note(op.id, "429")
			})
		}
	})

	assertDesktopStatuses(t, hit)
}

func assertDesktopStatuses(t *testing.T, hit map[string]map[string]bool) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepoRoot(t), "api", "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	var missing []string
	for _, item := range paths {
		ops, _ := item.(map[string]any)
		for method, raw := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete", "head":
			default:
				continue
			}
			op, _ := raw.(map[string]any)
			clients, _ := op["x-clients"].([]any)
			desktop := false
			for _, c := range clients {
				if c == "desktop" {
					desktop = true
				}
			}
			if !desktop {
				continue
			}
			id, _ := op["operationId"].(string)
			responses, _ := op["responses"].(map[string]any)
			for status := range responses {
				if !hit[id][status] {
					missing = append(missing, id+" "+status)
				}
			}
		}
	}
	if len(missing) > 0 {
		t.Fatalf("desktop contract statuses not exercised:\n%s", strings.Join(missing, "\n"))
	}
}

func assertErrorContract(t *testing.T, op string, body []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("%s error body: %v %s", op, err, body)
	}
	if _, ok := doc["error"].(string); !ok || doc["error"] == "" {
		t.Fatalf("%s legacy error missing: %s", op, body)
	}
	code, _ := doc["code"].(string)
	if code == "" || !knownErrorCode(code) {
		t.Fatalf("%s code %q", op, code)
	}
	if msg, _ := doc["message"].(string); strings.TrimSpace(msg) == "" {
		t.Fatalf("%s message missing", op)
	}
	if id, _ := doc["request_id"].(string); id == "" {
		t.Fatalf("%s request_id missing", op)
	}
}

func knownErrorCode(code string) bool {
	switch code {
	case "auth.unauthorized", "auth.invalid_credentials", "auth.session_expired",
		"auth.step_up_required", "auth.mfa_setup_required", "auth.mfa_required", "auth.mfa_token_invalid",
		"access.forbidden", "access.quota_exceeded", "sharing.policy_denied",
		"resource.not_found", "resource.conflict", "resource.gone", "multipart.upload_not_found",
		"request.invalid", "rate_limited", "setup.required", "setup.password_change_required", "internal":
		return true
	default:
		return false
	}
}

func callOn(t *testing.T, s *api.Server, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func jsonString(t *testing.T, body []byte, key string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json %s: %v body %s", key, err, body)
	}
	v, _ := doc[key].(string)
	if v == "" {
		t.Fatalf("missing %s in %s", key, body)
	}
	return v
}

func nestedString(t *testing.T, body []byte, obj, key string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	inner, _ := doc[obj].(map[string]any)
	v, _ := inner[key].(string)
	if v == "" {
		t.Fatalf("missing %s.%s in %s", obj, key, body)
	}
	return v
}

func challengeOf(t *testing.T, body []byte) string {
	t.Helper()
	var doc struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.PublicKey.Challenge == "" {
		t.Fatalf("no challenge in %s", body)
	}
	return doc.PublicKey.Challenge
}

func itoa(n int) string {
	return strconvItoa(n)
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func startFakeOIDC(t *testing.T) string {
	t.Helper()
	var issuer string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"userinfo_endpoint":      issuer + "/userinfo",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload, _ := json.Marshal(map[string]string{
			"preferred_username": "oidcdesk",
			"email":              "oidcdesk@example.com",
		})
		id := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": id,
			"token_type":   "Bearer",
			"id_token":     id,
		})
	})
	srv := httptest.NewServer(mux)
	issuer = srv.URL
	t.Cleanup(srv.Close)
	return issuer
}

type softKey struct {
	priv *ecdsa.PrivateKey
	cred webauthn.Credential
}

func newSoftKey(t *testing.T) softKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{KeyType: int64(webauthncose.EllipticKey), Algorithm: int64(webauthncose.AlgES256)},
		Curve:         int64(webauthncose.P256),
		XCoord:        priv.X.FillBytes(make([]byte, 32)),
		YCoord:        priv.Y.FillBytes(make([]byte, 32)),
	}
	raw, err := webauthncbor.Marshal(pub)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return softKey{priv: priv, cred: webauthn.Credential{ID: id, PublicKey: raw}}
}

func (k softKey) assertion(t *testing.T, challenge, origin, rpID string) []byte {
	t.Helper()
	clientData, _ := json.Marshal(map[string]any{
		"type":        "webauthn.get",
		"challenge":   challenge,
		"origin":      origin,
		"crossOrigin": false,
	})
	rpHash := sha256.Sum256([]byte(rpID))
	authData := append(rpHash[:], 0x05, 0, 0, 0, 1)
	clientHash := sha256.Sum256(clientData)
	sum := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	id := base64.RawURLEncoding.EncodeToString(k.cred.ID)
	body, _ := json.Marshal(map[string]any{
		"id": id, "rawId": id, "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientData),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(authData),
			"signature":         base64.RawURLEncoding.EncodeToString(sig),
			"userHandle":        base64.RawURLEncoding.EncodeToString([]byte("admin-bootstrap")),
		},
	})
	return body
}

func TestLoginIgnoresUnknownJSONField(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader([]byte(`{"username":"admin","password":"admin","future_field":{"n":1}}`)))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unknown field login %d %s", rec.Code, rec.Body.String())
	}
}
