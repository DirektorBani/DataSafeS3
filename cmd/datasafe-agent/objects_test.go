package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	if !step.StepUp || step.Code != "auth.step_up_required" || strings.Contains(step.Message, "web console") || calls.Load() != 2 {
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

func TestStepUpDeleteThenSecondAction(t *testing.T) {
	const totp = "654321"
	const recovery = "recovery-code-1"
	var mu sync.Mutex
	var hits []string
	seen := map[string]int{}
	var seq atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jwt" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/step-up":
			var posted map[string]string
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &posted)
			if posted["totp_code"] != "" || posted["code"] == "" {
				t.Errorf("step-up body %+v", posted)
			}
			n := seq.Add(1)
			token := "action-token-" + itoa(int(n))
			mu.Lock()
			hits = append(hits, "step-up "+posted["action"]+" "+posted["target"]+" "+posted["code"])
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": token, "action": posted["action"], "target": posted["target"],
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/buckets/plain":
			mu.Lock()
			hits = append(hits, "delete-plain "+r.Header.Get(mfaTokenHeader))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && (r.URL.Path == "/api/v1/buckets/photos" || r.URL.Path == "/api/v1/trash/t1"):
			header := r.Header.Get(mfaTokenHeader)
			action, target := "bucket_delete", "photos"
			if r.URL.Path == "/api/v1/trash/t1" {
				action = "trash_purge"
			}
			mu.Lock()
			hits = append(hits, r.URL.Path+" header="+header)
			if header != "" {
				seen[header]++
				if seen[header] > 1 {
					mu.Unlock()
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"mfa_token_invalid"}`))
					return
				}
			}
			mu.Unlock()
			if header == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "step_up_required", "action": action, "target": target})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
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
	params := func(extra map[string]any) map[string]any {
		out := map[string]any{"id": profile.ID, "token": "jwt"}
		for key, value := range extra {
			out[key] = value
		}
		return out
	}
	raw, rpcErr := app.dispatch("bucket.delete", params(map[string]any{"bucket": "plain", "step_up_code": totp}))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	plain := raw.(apiResult)
	if plain.Status != http.StatusNoContent || plain.StepUp {
		t.Fatalf("1.5-shaped delete %+v", plain)
	}
	mu.Lock()
	for _, hit := range hits {
		if strings.HasPrefix(hit, "step-up") {
			t.Fatalf("old server asked for a code: %v", hits)
		}
	}
	mu.Unlock()

	raw, rpcErr = app.dispatch("bucket.delete", params(map[string]any{"bucket": "photos"}))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	asked := raw.(apiResult)
	if !asked.StepUp || asked.Body["action"] != "bucket_delete" || asked.Body["target"] != "photos" {
		t.Fatalf("ask %+v", asked)
	}

	raw, rpcErr = app.dispatch("bucket.delete", params(map[string]any{"bucket": "photos", "step_up_code": totp}))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	deleted := raw.(apiResult)
	encoded, _ := json.Marshal(deleted)
	if deleted.Status != http.StatusNoContent || deleted.StepUp || strings.Contains(string(encoded), "action-token") || strings.Contains(string(encoded), totp) {
		t.Fatalf("delete %+v %s", deleted, encoded)
	}

	raw, rpcErr = app.dispatch("trash.purge", params(map[string]any{"trash_id": "t1"}))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	second := raw.(apiResult)
	if !second.StepUp || second.Body["action"] != "trash_purge" || second.Body["target"] != "photos" {
		t.Fatalf("second action reused a token: %+v", second)
	}
	mu.Lock()
	var purgeHeaders int
	for _, hit := range hits {
		if strings.HasPrefix(hit, "/api/v1/trash/t1") {
			purgeHeaders++
			if strings.Contains(hit, "action-token") {
				t.Fatalf("purge sent a stored token: %s", hit)
			}
		}
	}
	mu.Unlock()
	if purgeHeaders != 1 {
		t.Fatalf("purge calls %d", purgeHeaders)
	}
	raw, rpcErr = app.dispatch("trash.purge", params(map[string]any{"trash_id": "t1", "step_up_code": recovery}))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	purged := raw.(apiResult)
	encoded, _ = json.Marshal(purged)
	if purged.Status != http.StatusNoContent || strings.Contains(string(encoded), recovery) || strings.Contains(string(encoded), "action-token") {
		t.Fatalf("purge %+v %s", purged, encoded)
	}
	mu.Lock()
	joined := strings.Join(hits, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "step-up bucket_delete photos "+totp) || !strings.Contains(joined, "step-up trash_purge photos "+recovery) {
		t.Fatalf("step-up posts:\n%s", joined)
	}
}

func TestReplayedStepUpTokenOpensDialog(t *testing.T) {
	const totp = "111222"
	const token = "replayed-token"
	var presented atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/step-up":
			var posted map[string]string
			_ = json.NewDecoder(r.Body).Decode(&posted)
			if posted["code"] != totp || posted["action"] != "bucket_delete" || posted["target"] != "photos" {
				t.Errorf("posted %+v", posted)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token, "action": "bucket_delete", "target": "photos"})
		case r.Method == http.MethodDelete:
			if r.Header.Get(mfaTokenHeader) == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"step_up_required","action":"bucket_delete","target":"photos"}`))
				return
			}
			presented.Add(1)
			if r.Header.Get(mfaTokenHeader) != token {
				t.Errorf("header %q", r.Header.Get(mfaTokenHeader))
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"mfa_token_invalid"}`))
		default:
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
	raw, rpcErr := app.dispatch("bucket.delete", map[string]any{
		"id": profile.ID, "token": "jwt", "bucket": "photos", "step_up_code": totp,
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	result := raw.(apiResult)
	encoded, _ := json.Marshal(result)
	if !result.StepUp || result.Code != "auth.mfa_token_invalid" || presented.Load() != 1 {
		t.Fatalf("replay %+v presented %d", result, presented.Load())
	}
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), totp) {
		t.Fatalf("secret returned %s", encoded)
	}
}

func TestMFASetupRequiredIsABanner(t *testing.T) {
	var steps atomic.Int32
	var meCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/step-up" {
			steps.Add(1)
		}
		if r.URL.Path == "/api/v1/me" {
			if meCalls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"username":"alice","mfa_setup_required":true,"token":"jwt-should-stay","mfa_token":"mfa-should-stay"}`))
				return
			}
			_, _ = w.Write([]byte(`{"username":"alice","mfa_policy":{"banner":true,"applies":true,"has_local_factor":false},"token":"jwt-should-stay","mfa_token":"mfa-should-stay"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"mfa_setup_required","mfa_setup_required":true}`))
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
	raw, rpcErr := app.dispatch("bucket.delete", map[string]any{"id": profile.ID, "token": "jwt", "bucket": "photos", "step_up_code": "123456"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	blocked := raw.(apiResult)
	if !blocked.MFASetup || blocked.StepUp || steps.Load() != 0 {
		t.Fatalf("banner %+v steps %d", blocked, steps.Load())
	}
	raw, rpcErr = app.dispatch("account.me", map[string]any{"id": profile.ID, "token": "jwt"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	me := raw.(apiResult)
	encoded, _ := json.Marshal(me)
	if me.MFASetup || me.Body["username"] != "alice" || strings.Contains(string(encoded), "jwt-should-stay") || strings.Contains(string(encoded), "mfa-should-stay") {
		t.Fatalf("setup flag is not the banner %+v %s", me, encoded)
	}
	raw, rpcErr = app.dispatch("account.me", map[string]any{"id": profile.ID, "token": "jwt"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	me = raw.(apiResult)
	encoded, _ = json.Marshal(me)
	if !me.MFASetup || me.Body["username"] != "alice" || strings.Contains(string(encoded), "jwt-should-stay") || strings.Contains(string(encoded), "mfa-should-stay") {
		t.Fatalf("me %+v %s", me, encoded)
	}
}

func TestOfflineListCacheKeepsTheSavedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/buckets":
			_, _ = w.Write([]byte(`{"buckets":[{"name":"photos","encryption":{"wrapped_dek":"super-secret-dek"}}]}`))
		case "/api/v1/buckets/photos/objects":
			_, _ = w.Write([]byte(`{"objects":[{"key":"a.txt","size":4}],"truncated":false}`))
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
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"id": profile.ID, "token": "jwt", "bucket": "photos"}
	raw, rpcErr := app.dispatch("bucket.list", params)
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	listed := raw.(apiResult)
	if listed.Cached || listed.Status != http.StatusOK {
		t.Fatalf("live list %+v", listed)
	}
	if _, rpcErr = app.dispatch("object.list", params); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	if err := app.writeCache(profile.ID, ServerCard{ServerVersion: "1.6.1"}); err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(app.cachePath(profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disk), "super-secret-dek") || strings.Contains(string(disk), "jwt") {
		t.Fatal("list cache contains a secret")
	}
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"auth.session_expired","error":"session expired"}`))
	})
	raw, rpcErr = app.dispatch("bucket.list", params)
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	denied := raw.(apiResult)
	if denied.Cached || denied.Status != http.StatusUnauthorized {
		t.Fatalf("401 must not be served from cache %+v", denied)
	}
	srv.Close()
	raw, rpcErr = app.dispatch("object.list", params)
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	offline := raw.(apiResult)
	objects, _ := offline.Body["objects"].([]any)
	if !offline.Cached || offline.CachedAt == 0 || len(objects) != 1 {
		t.Fatalf("offline objects %+v", offline)
	}
	raw, rpcErr = app.dispatch("bucket.list", map[string]any{"id": profile.ID, "token": "jwt", "filter": "owned"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	other := raw.(apiResult)
	names, _ := other.Body["buckets"].([]any)
	if !other.Cached || len(names) != 1 {
		t.Fatalf("offline buckets %+v", other)
	}
	if err := app.deleteProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(app.cachePath(profile.ID)); !os.IsNotExist(err) {
		t.Fatal("forget should remove the list cache")
	}
}
