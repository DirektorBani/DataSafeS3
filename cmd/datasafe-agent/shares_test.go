package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSharePolicyBlock(t *testing.T) {
	day := map[string]any{"effective": map[string]any{
		"share_links_allowed": true,
		"presign_allowed":     true,
		"max_share_ttl_sec":   float64(86400),
		"max_presign_ttl_sec": float64(86400),
	}}
	if sharePolicyBlock(day, 86400, "", false) != nil {
		t.Fatal("a day is allowed")
	}
	if got := sharePolicyBlock(day, 86401, "", false); got == nil || got.Status != http.StatusBadRequest || got.Code != "max_share_ttl" {
		t.Fatalf("over max %+v", got)
	}
	if got := sharePolicyBlock(day, 0, "", false); got == nil || got.Code != "max_share_ttl" {
		t.Fatalf("unlimited %+v", got)
	}
	open := map[string]any{"effective": map[string]any{
		"share_links_allowed": true,
		"max_share_ttl_sec":   float64(0),
	}}
	if sharePolicyBlock(open, 0, "", false) != nil {
		t.Fatal("max 0 is not a cap")
	}
	if sharePolicyBlock(map[string]any{}, 999999, "", false) != nil {
		t.Fatal("missing effective is not a cap")
	}
	off := map[string]any{"effective": map[string]any{"share_links_allowed": false, "presign_allowed": true}}
	if got := sharePolicyBlock(off, 3600, "secret", false); got == nil || got.Status != http.StatusForbidden || got.Code != "sharing_disabled" {
		t.Fatalf("links off %+v", got)
	}
	if got := sharePolicyBlock(off, 3600, "", true); got == nil || got.Code != "sharing_disabled" {
		t.Fatalf("presign while links off %+v", got)
	}
	need := map[string]any{"effective": map[string]any{
		"share_links_allowed":     true,
		"share_password_required": true,
		"max_share_ttl_sec":       float64(86400),
	}}
	if got := sharePolicyBlock(need, 3600, "", false); got == nil || got.Status != http.StatusBadRequest || got.Code != "password_required" {
		t.Fatalf("password %+v", got)
	}
	if sharePolicyBlock(need, 3600, "once", false) != nil {
		t.Fatal("password present")
	}
	noPresign := map[string]any{"effective": map[string]any{
		"share_links_allowed": true,
		"presign_allowed":     false,
	}}
	if got := sharePolicyBlock(noPresign, 3600, "", true); got == nil || got.Code != "presign_disabled" {
		t.Fatalf("presign %+v", got)
	}
	if sharePolicyBlock(noPresign, 3600, "", false) != nil {
		t.Fatal("share link stays available when only presign is off")
	}
}

func TestShareCreateFollowsPolicy(t *testing.T) {
	var mu sync.Mutex
	var posts int
	var gets int
	var postedPassword string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sharing"):
			mu.Lock()
			gets++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"effective":{"share_links_allowed":false,"presign_allowed":false,"max_share_ttl_sec":86400,"share_password_required":true}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shares"):
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			mu.Lock()
			posts++
			postedPassword, _ = payload["password"].(string)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"share":{"id":"s1","token":"secret-token"},"url":"http://127.0.0.1/s/1","password":"share-secret-value"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := &App{dataDir: t.TempDir()}
	profile, err := app.putProfile(Profile{Name: "One", ServerURL: srv.URL, AuthMethod: "password", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Session = "signed_in"
	profile.Server = &ServerCard{Features: features160()}
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := app.dispatch("share.create", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "key": "a.txt",
		"expires_in_sec": 3600, "share_password": "share-secret-value",
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result := raw.(apiResult)
	if result.Status != http.StatusForbidden || result.Code != "sharing_disabled" || posts != 0 || gets != 1 {
		t.Fatalf("status %d code %s posts %d gets %d", result.Status, result.Code, posts, gets)
	}

	profile.Server = &ServerCard{Features: features160()}
	profile.Server.Features["sharing.policy"] = 1
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sharing"):
			mu.Lock()
			gets++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"effective":{"share_links_allowed":true,"share_password_required":true,"max_share_ttl_sec":86400}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shares"):
			mu.Lock()
			posts++
			mu.Unlock()
			t.Errorf("create without a password was posted")
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	})
	raw, rpcErr = app.dispatch("share.create", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "key": "a.txt", "expires_in_sec": 3600,
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result = raw.(apiResult)
	if result.Code != "password_required" || result.Status != http.StatusBadRequest {
		t.Fatalf("password %+v", result)
	}

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sharing"):
			_, _ = w.Write([]byte(`{"effective":{"share_links_allowed":true,"max_share_ttl_sec":86400}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shares"):
			t.Errorf("ttl over the cap was posted")
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	})
	raw, rpcErr = app.dispatch("share.create", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "key": "a.txt", "expires_in_sec": float64(7 * 86400),
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result = raw.(apiResult)
	if result.Code != "max_share_ttl" || result.Status != http.StatusBadRequest {
		t.Fatalf("ttl %+v", result)
	}

	profile.Server = &ServerCard{Features: features150()}
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	var legacyGets int
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sharing"):
			legacyGets++
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shares"):
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			postedPassword, _ = payload["password"].(string)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"share":{"id":"s1","token":"secret-token"},"url":"http://127.0.0.1/s/1","password":"share-secret-value"}`))
		default:
			http.NotFound(w, r)
		}
	})
	raw, rpcErr = app.dispatch("share.create", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "key": "a.txt",
		"expires_in_sec": float64(7 * 86400), "share_password": "share-secret-value",
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result = raw.(apiResult)
	encoded, _ := json.Marshal(result)
	if result.Status != http.StatusCreated || result.Body["url"] != "http://127.0.0.1/s/1" {
		t.Fatalf("1.5 create %+v", result)
	}
	if legacyGets != 0 {
		t.Fatalf("1.5 read policy %d", legacyGets)
	}
	if strings.Contains(string(encoded), "share-secret-value") || strings.Contains(string(encoded), "secret-token") {
		t.Fatalf("secret in result %s", encoded)
	}
	if postedPassword != "share-secret-value" {
		t.Fatalf("password was not posted")
	}
}
