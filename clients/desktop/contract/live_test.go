package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestLive runs table 4.6.6 against one server. desktop-compat sets
// DATASAFE_CONTRACT_URL. A plain go test skips it.
func TestLive(t *testing.T) {
	base := strings.TrimRight(os.Getenv("DATASAFE_CONTRACT_URL"), "/")
	if base == "" {
		t.Skip("DATASAFE_CONTRACT_URL is not set")
	}
	user := envOr("DATASAFE_CONTRACT_USER", "admin")
	pass := envOr("DATASAFE_CONTRACT_PASSWORD", "admin")
	want := os.Getenv("DATASAFE_CONTRACT_PROFILE")
	dev := os.Getenv("DATASAFE_CONTRACT_DEV") == "1"
	client := &http.Client{Timeout: 30 * time.Second}

	healthStatus, healthBody := call(t, client, http.MethodGet, base+"/healthz", "", nil)
	if healthStatus != 200 {
		t.Fatalf("GET /healthz -> %d", healthStatus)
	}
	health, ok := asObject(healthBody)
	if !ok {
		t.Fatal("healthz body is not an object")
	}
	version, _ := health["version"].(string)
	_, hasKMS := health["kms"]
	if dev {
		if version != "dev" {
			t.Fatalf("unstamped v1.5.0 healthz version is %q, want dev", version)
		}
		if hasKMS {
			t.Fatal("unstamped v1.5.0 healthz must not advertise kms")
		}
	}

	pubStatus, pubBody := call(t, client, http.MethodGet, base+"/api/v1/capabilities", "", nil)
	pub, _ := asObject(pubBody)
	loginStatus, loginBody := call(t, client, http.MethodPost, base+"/api/v1/admin/login", "", map[string]string{
		"username": user,
		"password": pass,
	})
	if loginStatus != 200 {
		t.Fatalf("POST /api/v1/admin/login -> %d", loginStatus)
	}
	login, ok := asObject(loginBody)
	if !ok {
		t.Fatal("login body is not an object")
	}
	token, _ := login["token"].(string)
	if token == "" {
		t.Fatal("login did not return a token")
	}

	authedStatus, authedBody := call(t, client, http.MethodGet, base+"/api/v1/capabilities", token, nil)
	authed, _ := asObject(authedBody)
	profile, features := Classify(pubStatus, pub, authedStatus, authed, version, hasKMS)
	t.Logf("profile %s version %s public_caps %d authed_caps %d", profile, version, pubStatus, authedStatus)
	serverVersion, _ := authed["server_version"].(string)
	if err := profileAllowed(want, profile, version, serverVersion); err != nil {
		t.Fatal(err)
	}

	bucket := "contract-e11"
	createStatus, _ := call(t, client, http.MethodPost, base+"/api/v1/buckets/"+bucket, token, map[string]any{})
	if createStatus != 200 && createStatus != 201 && createStatus != 409 {
		t.Fatalf("create bucket -> %d", createStatus)
	}
	payload := []byte("contract-e11")
	putStatus, _ := callRaw(t, client, http.MethodPut, base+"/api/v1/buckets/"+bucket+"/objects/contract.txt", token, "application/octet-stream", bytes.NewReader(payload))
	if putStatus != 200 && putStatus != 201 {
		t.Fatalf("PUT object -> %d", putStatus)
	}
	listStatus, listBody := call(t, client, http.MethodGet, base+"/api/v1/buckets/"+bucket+"/objects?max_keys=100", token, nil)
	if listStatus != 200 {
		t.Fatalf("GET objects -> %d", listStatus)
	}
	if listObj, ok := asObject(listBody); ok {
		if objects, exists := listObj["objects"]; exists && !isArray(objects) {
			t.Fatal("objects is not an array")
		}
	}
	gotStatus, gotBody := callRaw(t, client, http.MethodGet, base+"/api/v1/buckets/"+bucket+"/objects/contract.txt", token, "", nil)
	if gotStatus != 200 || !bytes.Equal(gotBody, payload) {
		t.Fatalf("GET object -> %d len %d", gotStatus, len(gotBody))
	}

	for _, row := range Matrix {
		on := features[row.Name] >= 1
		t.Run(row.Name, func(t *testing.T) {
			probe(t, client, base, token, bucket, row.Name, on)
		})
	}
}

// profileAllowed is the DATASAFE_CONTRACT_PROFILE check. 1.6.1 is released
// and answers capabilities with a features object, so its column is
// "features" plus healthz or server_version 1.6.1. "features" is that same
// profile on any version, which is what :main uses when the tip has moved on.
func profileAllowed(want, profile, version, serverVersion string) error {
	switch want {
	case "1.5.0", "1.6.0":
		if profile != want {
			return fmt.Errorf("profile %s, want %s", profile, want)
		}
	case "1.6.1":
		if profile != "features" {
			return fmt.Errorf("profile %s, want features", profile)
		}
		if !versionIs(version, "1.6.1") && !versionIs(serverVersion, "1.6.1") {
			return fmt.Errorf("version %s, want 1.6.1", version)
		}
	case "features":
		if profile != "features" {
			return fmt.Errorf("profile %s, want features", profile)
		}
	case "", "main":
	default:
		return fmt.Errorf("unknown DATASAFE_CONTRACT_PROFILE %q", want)
	}
	return nil
}

func versionIs(got, want string) bool {
	got = strings.TrimPrefix(strings.TrimSpace(got), "v")
	return got == want
}

func TestProfileValues(t *testing.T) {
	if err := profileAllowed("1.6.1", "features", "1.6.1", ""); err != nil {
		t.Fatal(err)
	}
	if err := profileAllowed("1.6.1", "features", "dev", "v1.6.1"); err != nil {
		t.Fatal(err)
	}
	if err := profileAllowed("1.6.1", "1.6.0", "1.6.1", "1.6.1"); err == nil {
		t.Fatal("1.6.1 must classify as features")
	}
	if err := profileAllowed("1.6.1", "features", "1.6.0", ""); err == nil {
		t.Fatal("1.6.1 requires version 1.6.1")
	}
	if err := profileAllowed("features", "features", "9.9.9", ""); err != nil {
		t.Fatal(err)
	}
	if err := profileAllowed("features", "1.6.0", "1.6.0", ""); err == nil {
		t.Fatal("features requires the features profile")
	}
	if err := profileAllowed("1.5.0", "1.5.0", "dev", ""); err != nil {
		t.Fatal(err)
	}
	if err := profileAllowed("main", "features", "1.6.1", ""); err != nil {
		t.Fatal(err)
	}
	if err := profileAllowed("nope", "features", "1.6.1", ""); err == nil {
		t.Fatal("unknown profile value")
	}
}

func probe(t *testing.T, client *http.Client, base, token, bucket, feature string, on bool) {
	t.Helper()
	switch feature {
	case "auth.password":
		if !on {
			t.Fatal("this suite signs in with a password, so auth.password is on")
		}
	case "auth.totp":
		status, _ := call(t, client, http.MethodPost, base+"/api/v1/mfa/login", "", map[string]any{})
		expectRoute(t, status, on)
	case "auth.oidc":
		// The route exists on 1.6.1 even when OIDC is not configured.
		// Table 4.6.6 says to hide OIDC when login-options omits it.
		status, body := call(t, client, http.MethodGet, base+"/api/v1/auth/login-options", "", nil)
		if on {
			expectRoute(t, status, true)
			return
		}
		if status == 404 {
			return
		}
		if status != 200 {
			t.Fatalf("login-options status %d", status)
		}
		if oidcOffered(body) {
			t.Fatal("login-options offers oidc while auth.oidc is off")
		}
	case "auth.refresh":
		status, _ := call(t, client, http.MethodPost, base+"/api/v1/auth/refresh", token, map[string]any{})
		expectRoute(t, status, on)
	case "objects.multipart":
		if !on {
			status, _ := call(t, client, http.MethodPost, base+"/api/v1/buckets/"+bucket+"/multipart", token, map[string]string{"key": "contract-part.bin"})
			if status != 404 {
				t.Fatalf("multipart route status %d, want 404", status)
			}
			return
		}
		status, body := call(t, client, http.MethodPost, base+"/api/v1/buckets/"+bucket+"/multipart", token, map[string]string{"key": "contract-part.bin"})
		if status != 200 {
			t.Fatalf("initiate multipart -> %d", status)
		}
		obj, _ := asObject(body)
		uploadID, _ := obj["upload_id"].(string)
		if uploadID == "" {
			t.Fatal("initiate multipart did not return upload_id")
		}
		abort := 0
		for i := 0; i < 3; i++ {
			abort, _ = call(t, client, http.MethodDelete, base+"/api/v1/buckets/"+bucket+"/multipart/"+uploadID, token, nil)
			if abort < 400 || abort == 404 {
				break
			}
		}
		if abort >= 400 && abort != 404 {
			t.Fatalf("abort multipart -> %d", abort)
		}
	case "objects.list_parts":
		status, _ := call(t, client, http.MethodGet, base+"/api/v1/buckets/"+bucket+"/multipart/missing/parts", token, nil)
		expectRoute(t, status, on)
	case "objects.range":
		req, err := http.NewRequest(http.MethodGet, base+"/api/v1/buckets/"+bucket+"/objects/contract.txt", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Range", "bytes=0-0")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if on && resp.StatusCode != http.StatusPartialContent {
			t.Fatalf("range status %d, want 206", resp.StatusCode)
		}
		if !on && resp.StatusCode == http.StatusPartialContent {
			t.Fatal("range returned 206 while objects.range is off")
		}
	case "sharing.links":
		status, body := call(t, client, http.MethodPost, base+"/api/v1/buckets/"+bucket+"/shares", token, map[string]any{
			"key":            "contract.txt",
			"expires_in_sec": 3600,
		})
		if !on {
			if status != 404 {
				t.Fatalf("share create status %d, want 404", status)
			}
			return
		}
		if status == 404 {
			t.Fatal("share create is 404 while sharing.links is on")
		}
		if status >= 400 && status != 403 {
			t.Fatalf("share create -> %d", status)
		}
		obj, _ := asObject(body)
		if id, _ := obj["id"].(string); id != "" {
			rev, _ := call(t, client, http.MethodDelete, base+"/api/v1/shares/"+id, token, nil)
			if rev != 204 && rev != 200 && rev != 404 {
				t.Fatalf("revoke share -> %d", rev)
			}
		}
	case "sharing.policy":
		status, _ := call(t, client, http.MethodGet, base+"/api/v1/buckets/"+bucket+"/sharing", token, nil)
		expectRoute(t, status, on)
	case "sharing.revoke_all":
		status, _ := call(t, client, http.MethodPost, base+"/api/v1/buckets/"+bucket+"/shares/revoke-all", token, map[string]any{})
		expectRoute(t, status, on)
	case "auth.step_up":
		status, _ := call(t, client, http.MethodPost, base+"/api/v1/auth/step-up", token, map[string]string{
			"action": "delete_bucket",
			"target": bucket,
			"code":   "000000",
		})
		expectRoute(t, status, on)
	case "encryption.kms":
		status, _ := call(t, client, http.MethodGet, base+"/api/v1/buckets/"+bucket+"/encryption", token, nil)
		expectRoute(t, status, on)
	case "errors.codes":
		status, body := call(t, client, http.MethodGet, base+"/api/v1/me", "", nil)
		if status != 401 {
			t.Fatalf("GET /api/v1/me without a token -> %d, want 401", status)
		}
		obj, ok := asObject(body)
		if !ok {
			t.Fatal("error body is not an object")
		}
		if _, ok := obj["error"].(string); !ok {
			t.Fatal("legacy error field is missing")
		}
		_, hasCode := obj["code"].(string)
		if on && !hasCode {
			t.Fatal("errors.codes is on but the 401 body has no code")
		}
		if !on && hasCode {
			t.Fatal("errors.codes is off but the 401 body has code")
		}
	case "server.secrets_stdin":
		if on {
			return
		}
		if _, present := healthKey(t, client, base, "secrets_stdin"); present {
			t.Fatal("healthz advertises secrets_stdin while the feature is off")
		}
	case "server.schema_version":
		if on {
			return
		}
		if _, present := healthKey(t, client, base, "schema_version"); present {
			t.Fatal("healthz advertises schema_version while the feature is off")
		}
	default:
		t.Fatalf("no probe for %s", feature)
	}
}

func oidcOffered(body []byte) bool {
	obj, ok := asObject(body)
	if !ok {
		return false
	}
	for _, key := range []string{"oidc", "oidc_enabled"} {
		if v, ok := obj[key].(bool); ok && v {
			return true
		}
	}
	for _, key := range []string{"methods", "login_methods"} {
		items, ok := obj[key].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			if s, ok := item.(string); ok && strings.EqualFold(s, "oidc") {
				return true
			}
		}
	}
	return false
}

func TestOIDCOffered(t *testing.T) {
	if oidcOffered([]byte(`{"local_login_enabled":true}`)) {
		t.Fatal("a local-login body does not offer oidc")
	}
	if !oidcOffered([]byte(`{"methods":["password","oidc"]}`)) {
		t.Fatal("a methods list can offer oidc")
	}
	if oidcOffered([]byte(`not json`)) {
		t.Fatal("a body that is not an object does not offer oidc")
	}
}

func expectRoute(t *testing.T, status int, on bool) {
	t.Helper()
	if on && status == 404 {
		t.Fatal("route is 404 while the feature is on")
	}
	if !on && status != 404 {
		t.Fatalf("route status %d, want 404", status)
	}
}

func healthKey(t *testing.T, client *http.Client, base, key string) (any, bool) {
	t.Helper()
	status, body := call(t, client, http.MethodGet, base+"/healthz", "", nil)
	if status != 200 {
		t.Fatalf("GET /healthz -> %d", status)
	}
	obj, ok := asObject(body)
	if !ok {
		t.Fatal("healthz body is not an object")
	}
	value, present := obj[key]
	return value, present
}

func call(t *testing.T, client *http.Client, method, url, token string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	kind := ""
	if payload != nil {
		kind = "application/json"
	}
	return callRaw(t, client, method, url, token, kind, body)
}

func callRaw(t *testing.T, client *http.Client, method, url, token, contentType string, body io.Reader) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode >= 500 {
		t.Fatalf("%s %s -> %d %s", method, pathOf(url), resp.StatusCode, redact(buf))
	}
	return resp.StatusCode, buf
}

func asObject(raw []byte) (map[string]any, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, true
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func pathOf(url string) string {
	if i := strings.Index(url, "/api/"); i >= 0 {
		return url[i:]
	}
	if i := strings.Index(url, "/healthz"); i >= 0 {
		return url[i:]
	}
	return url
}

var secretValue = regexp.MustCompile(`(?i)"(token|password|secret|authorization|signature|request_id)"\s*:\s*"[^"]*"`)

func redact(body []byte) string {
	text := secretValue.ReplaceAllString(string(body), `"$1":"<masked>"`)
	if len(text) > 240 {
		text = text[:240]
	}
	return text
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
