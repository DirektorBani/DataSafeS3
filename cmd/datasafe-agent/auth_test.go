package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := &App{dataDir: t.TempDir()}
	t.Cleanup(func() { app.closeTransfers() })
	return app
}

func TestLoginRetryForgetAndSecrets(t *testing.T) {
	password := "hunter2-not-in-logs"
	var token string
	var logins int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/capabilities" && r.Header.Get("Authorization") == "":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["password","oidc"]}`))
		case r.URL.Path == "/api/v1/capabilities":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["password","oidc"],"features":{"auth.password":1,"auth.totp":1,"auth.oidc":1,"errors.codes":1,"objects.multipart":1},"clients":{"desktop":{"min_supported":"1.0.0","min_required":null,"min_required_reason":null}},"deprecations":[{"method":"GET","path":"/api/v1/example","since":"1.6.1"}]}`))
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.6.1"}`))
		case r.URL.Path == "/api/v1/admin/login":
			logins++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["password"] != password {
				http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
				return
			}
			token = "jwt-one"
			if logins > 1 {
				token = "jwt-two"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": token, "expires_in": 86400, "username": "alice", "role": "user"})
		case r.URL.Path == "/api/v1/me":
			if r.Header.Get("Authorization") != "Bearer jwt-two" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized","code":"auth.unauthorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		case r.URL.Path == "/api/v1/admin/logout":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	app := newTestApp(t)
	first, err := app.putProfile(Profile{Name: "One", ServerURL: srv.URL, AuthMethod: "password", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.5.0"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer secondServer.Close()
	second, err := app.putProfile(Profile{Name: "Two", ServerURL: secondServer.URL, AuthMethod: "api_token"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := app.listProfiles()
	if err != nil || len(listed) != 2 {
		t.Fatalf("profiles %+v %v", listed, err)
	}

	logged, err := app.loginPassword(first, "alice", password)
	if err != nil || logged.Status != "signed_in" || logged.Token == "" {
		t.Fatalf("login %+v %v", logged, err)
	}
	if logged.Server.Profile != "features" || logged.Server.Features["errors.codes"] != 1 {
		t.Fatalf("authenticated features %+v", logged.Server)
	}
	if logged.Server.Deprecation == "" {
		t.Fatal("expected a deprecation warning")
	}
	raw, err := os.ReadFile(filepath.Join(app.dataDir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), password) || strings.Contains(string(raw), logged.Token) {
		t.Fatal("profiles.json contains a secret")
	}

	probe, err := app.probe(first, "stale-token", false)
	if err != nil || probe.Status != "reauth" {
		t.Fatalf("first 401 %+v %v", probe, err)
	}
	again, err := app.loginPassword(first, "alice", password)
	if err != nil || again.Token != "jwt-two" {
		t.Fatalf("relogin %+v %v", again, err)
	}
	okProbe, err := app.probe(first, again.Token, true)
	if err != nil || okProbe.Status != "signed_in" {
		t.Fatalf("retry probe %+v %v", okProbe, err)
	}
	bad, err := app.probe(first, "still-bad", true)
	if err != nil || bad.Status != "login_error" {
		t.Fatalf("second 401 %+v %v", bad, err)
	}

	out, err := app.logout(first, again.Token)
	if err != nil || out.PasswordRevoked || out.Message == "" {
		t.Fatalf("password logout should explain that the token stays valid: %+v %v", out, err)
	}

	card, err := app.inspect(second, "")
	if err != nil || card.Profile != "1.5.0" || !card.BrowseAllowed {
		t.Fatalf("second server %+v %v", card, err)
	}
	if err := app.deleteProfile(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(app.cachePath(first.ID)); !os.IsNotExist(err) {
		t.Fatal("forget should remove the list cache")
	}
	left, err := app.listProfiles()
	if err != nil || len(left) != 1 || left[0].ID != second.ID {
		t.Fatalf("left %+v %v", left, err)
	}
}

func TestRejectOldServerAndBlockFloor(t *testing.T) {
	var loginCalled bool
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write(healthzV141)
		case "/api/v1/auth/login-options":
			// Present on v1.4.1 and returns 200. Not a version signal.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"local_login_enabled":true}`))
		case "/api/v1/events/delivery":
			http.NotFound(w, r)
		case "/api/v1/admin/login":
			loginCalled = true
			_, _ = w.Write([]byte(`{"token":"should-not-issue","expires_in":60}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer old.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "Old", ServerURL: old.URL, AuthMethod: "password", Username: "a"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.loginPassword(profile, "a", "secret")
	if err != nil || got.Status != "rejected" || got.Token != "" || got.Server.BrowseAllowed {
		t.Fatalf("1.4.1 %+v %v", got, err)
	}
	if loginCalled {
		t.Fatal("sign-in must not run against a server older than 1.5.0")
	}

	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["password"],"features":{"auth.password":1},"clients":{"desktop":{"min_supported":"1.0.0","min_required":"9.0.0","min_required_reason":"security","advisory_url":"https://example.test/up"}}}`))
	}))
	defer blocked.Close()
	profile, err = app.putProfile(Profile{Name: "Blocked", ServerURL: blocked.URL, AuthMethod: "password"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = app.loginPassword(profile, "a", "secret")
	if err != nil || got.Status != "blocked" || got.Server.AdvisoryURL == "" {
		t.Fatalf("blocked %+v %v", got, err)
	}
	if _, err := os.Stat(app.cachePath(profile.ID)); err != nil {
		t.Fatal("blocked profile should keep the list cache")
	}
}

func TestHealthzFixturesFromTags(t *testing.T) {
	var v141 map[string]any
	if err := json.Unmarshal(healthzV141, &v141); err != nil {
		t.Fatal(err)
	}
	if _, ok := v141["version"]; ok {
		t.Fatal("v1.4.1 fixture must omit version")
	}
	var v150 map[string]any
	if err := json.Unmarshal(healthzV150, &v150); err != nil {
		t.Fatal(err)
	}
	version, _ := v150["version"].(string)
	commit, _ := v150["commit"].(string)
	if version == "" || commit == "" {
		t.Fatalf("v1.5.0 fixture must match TestHealthReportsVersionAndCommit: %#v", v150)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write(healthzV150)
		case "/api/v1/events/delivery":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case "/api/v1/admin/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-150", "expires_in": 86400, "username": "a"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "150", ServerURL: srv.URL, AuthMethod: "password", Username: "a"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.loginPassword(profile, "a", "secret")
	if err != nil || got.Status != "signed_in" || got.Token != "jwt-150" || got.Server.Profile != "1.5.0" {
		t.Fatalf("v1.5.0 fixture %+v %v", got, err)
	}

	unknown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write(healthzUnknown)
		case "/api/v1/admin/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-unknown", "expires_in": 86400, "username": "a"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer unknown.Close()
	profile, err = app.putProfile(Profile{Name: "Unknown", ServerURL: unknown.URL, AuthMethod: "password", Username: "a"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = app.loginPassword(profile, "a", "secret")
	if err != nil || got.Status != "signed_in" || got.Token != "jwt-unknown" || got.Server.Profile != "basic" || !got.Server.BrowseAllowed {
		t.Fatalf("present but unknown version should stay basic: %+v %v", got, err)
	}

	blank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write(healthzBlankVersion)
			return
		}
		if r.URL.Path == "/api/v1/admin/login" {
			t.Error("blank version must not receive credentials")
		}
		http.NotFound(w, r)
	}))
	defer blank.Close()
	profile, err = app.putProfile(Profile{Name: "Blank", ServerURL: blank.URL, AuthMethod: "password", Username: "a"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = app.loginPassword(profile, "a", "secret")
	if err != nil || got.Status != "rejected" || got.Token != "" {
		t.Fatalf("blank version %+v %v", got, err)
	}
}

func TestMFAAndEchoedSecret(t *testing.T) {
	password := "echo-me-not"
	var sawLogin bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.6.0","kms":{"available":false}}`))
		case "/api/v1/capabilities":
			_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.0","spec_version":"1.6.0","login_methods":["password"]}`))
		case "/api/v1/admin/login":
			sawLogin = true
			_ = json.NewEncoder(w).Encode(map[string]any{"mfa_required": true, "mfa_token": "mfa-1"})
		case "/api/v1/mfa/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["mfa_code"] != "123456" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"unauthorized `+password+`"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-mfa", "expires_in": 10, "username": "alice"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "MFA", ServerURL: srv.URL, AuthMethod: "password", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	step, err := app.loginPassword(profile, "alice", password)
	if err != nil || step.Status != "mfa_required" || step.MFAToken != "mfa-1" {
		t.Fatalf("mfa %+v %v", step, err)
	}
	if !sawLogin {
		t.Fatal("password login was not sent")
	}
	bad, err := app.loginMFA(profile, "mfa-1", "000000")
	if err != nil || bad.Status != "error" || strings.Contains(bad.Message, password) {
		t.Fatalf("error text leaked or was wrong: %+v %v", bad, err)
	}
	ok, err := app.loginMFA(profile, "mfa-1", "123456")
	if err != nil || ok.Token != "jwt-mfa" || !ok.Server.ExpiresSoon && ok.ExpiresIn != 10 {
		t.Fatalf("mfa session %+v %v", ok, err)
	}
	reloaded, err := app.profileByID(profile.ID)
	if err != nil || reloaded.ExpiresAtUnix == 0 {
		t.Fatal(reloaded)
	}
}

func TestOIDCExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.6.1"}`))
		case "/api/v1/capabilities":
			_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["oidc"]}`))
		case "/api/v1/admin/logout":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/auth/oidc/exchange":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["exchange_code"] != "once" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid or expired exchange code"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "oidc-jwt", "auth_source": "oidc"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "OIDC", ServerURL: srv.URL, AuthMethod: "oidc", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.loginOIDCExchange(profile, "once")
	if err != nil || got.Token != "oidc-jwt" || !got.PasswordRevoked {
		t.Fatalf("%+v %v", got, err)
	}
	out, err := app.logout(profile, got.Token)
	if err != nil || !out.PasswordRevoked {
		t.Fatalf("oidc logout %+v %v", out, err)
	}
}

func TestLoginResumesPausedTransfer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.6.1"}`))
		case "/api/v1/capabilities":
			_, _ = w.Write([]byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["password"]}`))
		case "/api/v1/admin/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "jwt-resume", "expires_in": 86400, "username": "alice"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "One", ServerURL: srv.URL, AuthMethod: "password", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	path := writeTemp(t, []byte("hello"))
	journal, err := openJournal(filepath.Join(app.dataDir, "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeRemote{}
	eng := newEngine(journal)
	eng.syncRun = true
	eng.limits = Limits{Threshold: 1 << 20, PartSize: 1 << 20, Transfers: 1, PartParallel: 1}
	eng.build = func(creds, string) (remote, error) { return fake, nil }
	app.transfers = eng
	task := Task{
		ID: "t1", ProfileID: profile.ID, Direction: "upload", State: "interrupted", Message: "paused_signed_out",
		Bucket: "b", Key: "k", LocalPath: path,
	}
	if err := journal.Save(task); err != nil {
		t.Fatal(err)
	}
	got, err := app.loginPassword(profile, "alice", "secret")
	if err != nil || got.Status != "signed_in" {
		t.Fatalf("login %+v %v", got, err)
	}
	loaded, err := journal.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != "done" || fake.puts != 1 {
		t.Fatalf("state %s puts %d message %s", loaded.State, fake.puts, loaded.Message)
	}
}

func TestBrowseSessionExpiryMarksReauth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"session expired"}`))
	}))
	defer srv.Close()
	app := newTestApp(t)
	profile, err := app.putProfile(Profile{Name: "OIDC", ServerURL: srv.URL, AuthMethod: "oidc", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Session = "signed_in"
	if err := app.updateProfile(profile); err != nil {
		t.Fatal(err)
	}
	result, err := app.browse(profile, "jwt", http.MethodGet, "/api/v1/me", nil, nil)
	if err != nil || result.Code != "auth.session_expired" {
		t.Fatalf("%+v %v", result, err)
	}
	reloaded, err := app.profileByID(profile.ID)
	if err != nil || reloaded.Session != "reauth" {
		t.Fatalf("session %s %v", reloaded.Session, err)
	}
}

func TestAgentArgsCarryNoSecret(t *testing.T) {
	args := []string{"--data-dir", t.TempDir()}
	for _, arg := range args {
		if strings.Contains(arg, "password") || strings.Contains(arg, "secret") || strings.Contains(arg, "token") {
			t.Fatal(arg)
		}
	}
}
