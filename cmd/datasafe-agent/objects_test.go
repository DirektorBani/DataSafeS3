package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestObjectListStaysOnOnePage(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/buckets/photos/objects" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer jwt" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("max_keys") != "100" || r.URL.Query().Get("delimiter") != "/" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"objects":[{"key":"a","size":1,"encryption":{"wrapped_dek":"super-secret-dek","sse_algorithm":"AES256"}}],"folders":["album/"],"truncated":true,"next_marker":"p2"}`))
	}))
	defer srv.Close()
	app := &App{dataDir: t.TempDir()}
	profile, err := app.putProfile(Profile{Name: "One", ServerURL: srv.URL, AuthMethod: "password", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Session = "signed_in"
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := app.dispatch("object.list", map[string]any{"id": profile.ID, "token": "jwt", "bucket": "photos"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	if calls.Load() != 1 {
		t.Fatalf("agent followed the next page: %d calls", calls.Load())
	}
	encoded, _ := json.Marshal(raw)
	if strings.Contains(string(encoded), "super-secret-dek") {
		t.Fatal("wrapped key material reached the browser result")
	}
	result := raw.(apiResult)
	if result.Status != http.StatusOK || result.Body["truncated"] != true || result.Body["next_marker"] != "p2" {
		t.Fatalf("%+v", result)
	}

	calls.Store(0)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("start_after") != "p2" {
			t.Errorf("marker %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"objects":[{"key":"b"}],"truncated":false,"next_marker":""}`))
	})
	_, rpcErr = app.dispatch("object.list", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "start_after": "p2",
	})
	if rpcErr != nil || calls.Load() != 1 {
		t.Fatalf("next page %v calls %d", rpcErr, calls.Load())
	}
}

func TestBrowseForbiddenAndStepUp(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/v1/buckets/ro/bulk-delete":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		case "/api/v1/buckets/locked":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"step_up_required","code":"auth.step_up_required"}`))
		default:
			if r.URL.Path == "/api/v1/admin/login" {
				t.Error("browse sent credentials")
			}
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := &App{dataDir: t.TempDir()}
	profile, err := app.putProfile(Profile{Name: "One", ServerURL: srv.URL, AuthMethod: "password"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Session = "signed_in"
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := app.dispatch("object.delete", map[string]any{"id": profile.ID, "token": "jwt", "bucket": "ro", "key": "a"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	denied := raw.(apiResult)
	if denied.Status != http.StatusForbidden || denied.Code != "access.forbidden" {
		t.Fatalf("403 %+v", denied)
	}
	raw, rpcErr = app.dispatch("bucket.delete", map[string]any{"id": profile.ID, "token": "jwt", "bucket": "locked"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	step := raw.(apiResult)
	if !step.StepUp || !strings.Contains(step.Message, "web console") || calls.Load() != 2 {
		t.Fatalf("step-up %+v calls %d", step, calls.Load())
	}
}

func TestBrowseRefusesOldProfileWithoutHTTP(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	app := &App{dataDir: t.TempDir()}
	profile, err := app.putProfile(Profile{Name: "Old", ServerURL: srv.URL, AuthMethod: "password"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Session = "rejected"
	profile.Server = &ServerCard{Rejected: true, Profile: "rejected"}
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := app.dispatch("bucket.list", map[string]any{"id": profile.ID, "token": "jwt"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result := raw.(apiResult)
	if result.Code != "server.too_old" || calls.Load() != 0 {
		t.Fatalf("%+v calls %d", result, calls.Load())
	}
}
