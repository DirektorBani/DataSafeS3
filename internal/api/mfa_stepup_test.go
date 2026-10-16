package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/pquerna/otp/totp"
)

func putMFAPolicy(t *testing.T, s interface{ Handler() http.Handler }, token string, grace int, tenant, global bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"require_bucket_admin_mfa": true,
		"grace_period_days":        grace,
		"apply_to_tenant_admins":   tenant,
		"apply_to_global_admins":   global,
	})
	req := authReq(http.MethodPut, "/api/v1/settings/mfa-policy", token, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mfa policy %d %s", rec.Code, rec.Body.String())
	}
}

func seedUser(t *testing.T, s interface {
	Meta() metadata.MetadataStore
}, username, password string) metadata.UserRecord {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	rec := metadata.UserRecord{
		ID: username + "-id", Username: username, PasswordHash: hash,
		Role: metadata.RoleUser, Status: metadata.StatusActive, CreatedAt: time.Now().UTC(),
	}
	if err := s.Meta().PutUser(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func enrollTOTP(t *testing.T, h http.Handler, token string) (string, []string) {
	t.Helper()
	req := authReq(http.MethodPost, "/api/v1/mfa/enroll", token, []byte(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	secret, _ := body["secret"].(string)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"code": code})
	req = authReq(http.MethodPost, "/api/v1/mfa/verify-enroll", token, raw)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify enroll %d %s", rec.Code, rec.Body.String())
	}
	var verified map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &verified)
	var recovery []string
	if list, ok := verified["recovery_codes"].([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				recovery = append(recovery, s)
			}
		}
	}
	return secret, recovery
}

func stepUpCode(t *testing.T, h http.Handler, token, code, action, target string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"action": action, "target": target, "totp_code": code})
	req := authReq(http.MethodPost, "/api/v1/auth/step-up", token, raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("step-up %s %d %s", action, rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == "" {
		t.Fatal("empty action token")
	}
	return body["token"]
}

func stepUp(t *testing.T, h http.Handler, token, secret, action, target string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"action": action, "target": target, "totp_code": code})
	req := authReq(http.MethodPost, "/api/v1/auth/step-up", token, raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("step-up %s %d %s", action, rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == "" {
		t.Fatal("empty action token")
	}
	return body["token"]
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body []byte, mfa string) *httptest.ResponseRecorder {
	t.Helper()
	req := authReq(method, path, token, body)
	if mfa != "" {
		req.Header.Set("X-DataSafe-MFA-Token", mfa)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBucketAdminMFAGraceAndStepUp(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	seedUser(t, s, "ownermfa", "owner-pass-1")
	seedUser(t, s, "othermfa", "other-pass-1")
	ownerTok := loginToken(t, s, "ownermfa", "owner-pass-1")
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/mfabox", ownerTok, []byte(`{}`), "")
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}
	putMFAPolicy(t, s, adminTok, 0, false, false)
	putShareWebhook(t, s)

	grants := []byte(`{"grants":[]}`)
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("grants without mfa %d %s", rec.Code, rec.Body.String())
	}
	var denied map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &denied)
	if denied["error"] != "mfa_required" {
		t.Fatalf("error %v", denied)
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("second deny %d", rec.Code)
	}
	if n := countOutboxType(t, s, "MfaRequiredDenied"); n != 1 {
		t.Fatalf("MfaRequiredDenied events %d, want 1", n)
	}
	if n := countActivity(t, s, metadata.ActionMFARequiredDenied); n != 1 {
		t.Fatalf("activity %d, want 1", n)
	}

	secret, recovery := enrollTOTP(t, s.Handler(), ownerTok)
	if len(recovery) < 3 {
		t.Fatalf("recovery codes %d", len(recovery))
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("step-up missing %d %s", rec.Code, rec.Body.String())
	}
	var step map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &step)
	if step["error"] != "step_up_required" || step["action"] != auth.ActionGrantsChange {
		t.Fatalf("step body %v", step)
	}
	actionTok := stepUp(t, s.Handler(), ownerTok, secret, auth.ActionGrantsChange, "mfabox", time.Now().Add(30*time.Second))
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, actionTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("grants with token %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, actionTok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replay %d %s", rec.Code, rec.Body.String())
	}

	otherTok := stepUpCode(t, s.Handler(), ownerTok, recovery[0], auth.ActionGrantsChange, "otherbox")
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, otherTok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong target %d %s", rec.Code, rec.Body.String())
	}
	delTok := stepUpCode(t, s.Handler(), ownerTok, recovery[1], auth.ActionBucketDelete, "mfabox")
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", ownerTok, grants, delTok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong action %d %s", rec.Code, rec.Body.String())
	}
	stolen := stepUpCode(t, s.Handler(), ownerTok, recovery[2], auth.ActionGrantsChange, "mfabox")
	otherLogin := loginToken(t, s, "othermfa", "other-pass-1")
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mfabox/access", otherLogin, grants, stolen)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("other user token %d %s", rec.Code, rec.Body.String())
	}

	past := time.Now().UTC().Add(-2 * time.Minute)
	if err := s.Meta().PutMFAActionToken(metadata.MFAActionToken{
		Hash: "unused", UserID: "x", ExpiresAt: past,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMFAGraceWindow(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	seedUser(t, s, "graceowner", "grace-pass-1")
	tok := loginToken(t, s, "graceowner", "grace-pass-1")
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/gracebox", tok, []byte(`{}`), "")
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("bucket %d %s", rec.Code, rec.Body.String())
	}
	putMFAPolicy(t, s, adminTok, 7, false, false)
	grants := []byte(`{"grants":[]}`)
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/gracebox/access", tok, grants, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("during grace %d %s", rec.Code, rec.Body.String())
	}
	me := doJSON(t, s.Handler(), http.MethodGet, "/api/v1/me", tok, nil, "")
	var meBody map[string]any
	_ = json.Unmarshal(me.Body.Bytes(), &meBody)
	policy, _ := meBody["mfa_policy"].(map[string]any)
	if policy["banner"] != true || policy["in_grace"] != true {
		t.Fatalf("banner during grace: %v", policy)
	}
	cfg, _ := s.Meta().GetSystemConfig()
	past := time.Now().UTC().Add(-8 * 24 * time.Hour)
	cfg.MFA.EnabledAt = &past
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	owner, err := s.Meta().GetUserByUsername("graceowner")
	if err != nil {
		t.Fatal(err)
	}
	owner.BucketAdminSince = &past
	if err := s.Meta().UpdateUser(owner); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/gracebox/access", tok, grants, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("after grace %d %s", rec.Code, rec.Body.String())
	}
}

func TestTOTPStepRejectedTwice(t *testing.T) {
	s := testServer(t)
	seedUser(t, s, "totpuser", "totp-pass-1")
	tok := loginToken(t, s, "totpuser", "totp-pass-1")
	secret, _ := enrollTOTP(t, s.Handler(), tok)
	code, err := totp.GenerateCode(secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "totpuser", "password": "totp-pass-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var login map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	if login["mfa_required"] != true {
		t.Fatalf("login %+v", login)
	}
	mfaBody, _ := json.Marshal(map[string]string{"mfa_token": login["mfa_token"].(string), "totp_code": code})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/mfa/login", bytes.NewReader(mfaBody))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mfa login %d %s", rec.Code, rec.Body.String())
	}
	again, _ := json.Marshal(map[string]string{"action": auth.ActionAccessKeyCreate, "target": "totpuser-id", "totp_code": code})
	var session map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &session)
	req = authReq(http.MethodPost, "/api/v1/auth/step-up", session["token"], again)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("same totp step accepted twice")
	}
}

func TestWebAuthnStillRequiredAfterTOTPDisable(t *testing.T) {
	s := testServer(t)
	user := seedUser(t, s, "wakey", "wakey-pass-1")
	user.WebAuthnCredentials = `[{"id":"a2V5LTE="}]`
	user.MFAEnabled = false
	if err := s.Meta().UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "wakey", "password": "wakey-pass-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var login map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	if login["token"] != nil && login["token"] != "" {
		t.Fatalf("password login issued a session while a security key remains: %v", login)
	}
	if login["mfa_required"] != true || login["webauthn_required"] != true {
		t.Fatalf("login %+v", login)
	}
}

func TestMFAResetRequiresTokenAndReason(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	secret, _ := enrollTOTP(t, s.Handler(), adminTok)
	putMFAPolicy(t, s, adminTok, 0, false, true)
	putShareWebhook(t, s)
	victim := seedUser(t, s, "victim", "victim-pass-1")
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/users/"+victim.ID+"/mfa/reset", adminTok, []byte(`{"reason":"lost device"}`), "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reset without token %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodPost, "/api/v1/users/"+victim.ID+"/mfa/reset", adminTok, []byte(`{}`), "nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reset without reason %d %s", rec.Code, rec.Body.String())
	}
	tok := stepUp(t, s.Handler(), adminTok, secret, auth.ActionMFAReset, victim.ID, time.Now().Add(30*time.Second))
	rec = doJSON(t, s.Handler(), http.MethodPost, "/api/v1/users/"+victim.ID+"/mfa/reset", adminTok, []byte(`{"reason":"lost device"}`), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset %d %s", rec.Code, rec.Body.String())
	}
	if n := countActivity(t, s, metadata.ActionMFAReset); n != 1 {
		t.Fatalf("activity %d", n)
	}
	if n := countOutboxType(t, s, "MfaReset"); n != 1 {
		t.Fatalf("event %d", n)
	}
}

func TestActionTokenConsumedOnceUnderRace(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	seedUser(t, s, "raceowner", "race-pass-1")
	tok := loginToken(t, s, "raceowner", "race-pass-1")
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/racebox", tok, []byte(`{}`), "")
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("bucket %d %s", rec.Code, rec.Body.String())
	}
	putMFAPolicy(t, s, adminTok, 0, false, false)
	secret, _ := enrollTOTP(t, s.Handler(), tok)
	actionTok := stepUp(t, s.Handler(), tok, secret, auth.ActionGrantsChange, "racebox", time.Now().Add(30*time.Second))
	grants := []byte(`{"grants":[]}`)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/racebox/access", tok, grants, actionTok)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()
	okN := 0
	for _, c := range codes {
		if c == http.StatusOK {
			okN++
		}
	}
	if okN != 1 {
		t.Fatalf("parallel consume codes %v, want one 200", codes)
	}
}

func TestAccessKeyCreatedWithMFAGatesAdminS3(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	seedUser(t, s, "keyowner", "key-pass-1")
	tok := loginToken(t, s, "keyowner", "key-pass-1")
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/keybox", tok, []byte(`{}`), "")
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("bucket %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodPost, "/api/v1/keys", tok, []byte(`{"label":"old"}`), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("key %d %s", rec.Code, rec.Body.String())
	}
	var oldKey map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &oldKey)
	putMFAPolicy(t, s, adminTok, 0, false, false)

	enc := []byte(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	if code := signedS3(t, s, oldKey["access_key"], oldKey["secret_key"], http.MethodPut, "/keybox?encryption", enc); code != http.StatusForbidden {
		t.Fatalf("old key encryption %d", code)
	}
	if code := signedS3(t, s, oldKey["access_key"], oldKey["secret_key"], http.MethodPut, "/keybox/a.txt", []byte("hi")); code != http.StatusOK {
		t.Fatalf("old key put object %d", code)
	}
	secret, _ := enrollTOTP(t, s.Handler(), tok)
	mfa := stepUp(t, s.Handler(), tok, secret, auth.ActionAccessKeyCreate, "keyowner-id", time.Now().Add(30*time.Second))
	req := authReq(http.MethodPost, "/api/v1/keys", tok, []byte(`{"label":"new"}`))
	req.Header.Set("X-DataSafe-MFA-Token", mfa)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("new key %d %s", rec.Code, rec.Body.String())
	}
	var newKey map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &newKey)
	if code := signedS3(t, s, newKey["access_key"], newKey["secret_key"], http.MethodPut, "/keybox?encryption", enc); code != http.StatusOK {
		t.Fatalf("new key encryption %d", code)
	}
}

func signedS3(t *testing.T, s interface{ Handler() http.Handler }, access, secret, method, path string, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "localhost"
	if err := auth.SignRequest(req, auth.Credentials{AccessKey: access, SecretKey: secret}, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code >= 400 {
		b, _ := io.ReadAll(rec.Body)
		t.Logf("s3 %s %s -> %d %s", method, path, rec.Code, b)
	}
	return rec.Code
}

func TestIdPFactorFromAMR(t *testing.T) {
	if !auth.IdPFactorAccepted([]string{"mfa"}, "", nil, nil) {
		t.Fatal("amr mfa")
	}
	if auth.IdPFactorAccepted(nil, "", nil, nil) {
		t.Fatal("missing amr")
	}
}
