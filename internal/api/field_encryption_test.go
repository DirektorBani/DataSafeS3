package api_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

func testServerWithFieldEnc(t *testing.T, enabled bool) *api.Server {
	t.Helper()
	if _, ok := os.LookupEnv("STORAGE_AUTO_HOME_BUCKET"); !ok {
		t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	fe, err := fieldenc.NewForTest("kek-api-test", seed, enabled)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s, err := api.NewServer(api.Config{
		DataDir:         dir,
		AdminUser:       "admin",
		AdminPassword:   "admin",
		JWTSecret:       "test",
		FieldEncryption: fe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFieldEncryptionAPI_securityStatusDisabled(t *testing.T) {
	s := testServerWithFieldEnc(t, false)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/settings/security-status", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		FieldEncryption struct {
			Enabled       bool `json:"enabled"`
			RegistryCount int  `json:"registry_count"`
		} `json:"field_encryption"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.FieldEncryption.Enabled {
		t.Fatal("expected field_encryption.enabled false")
	}
	if resp.FieldEncryption.RegistryCount != 0 {
		t.Fatalf("registry_count %d want 0 when disabled", resp.FieldEncryption.RegistryCount)
	}
}

func TestFieldEncryptionAPI_accessKeyS3RoundtripDisabled(t *testing.T) {
	s := testServerWithFieldEnc(t, false)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/fe-plain-bucket", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	keyBody, _ := json.Marshal(map[string]string{"label": "fe-plain"})
	req = authReq(http.MethodPost, "/api/v1/keys", adminTok, keyBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key %d %s", rec.Code, rec.Body.String())
	}
	var keyResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &keyResp)
	ak := keyResp["access_key"]
	sk := keyResp["secret_key"]

	reporter, ok := s.Meta().(metadata.FieldEncryptionReporter)
	if !ok {
		t.Fatal("metadata store should implement FieldEncryptionReporter")
	}
	if reporter.FieldEnc().Enabled() {
		t.Fatal("field encryption should be disabled on store")
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/fe-plain-bucket/plain-object.txt", bytes.NewReader([]byte("plain-payload")))
	putReq.Header.Set("Content-Type", "text/plain")
	creds := auth.Credentials{AccessKey: ak, SecretKey: sk}
	if err := auth.SignRequest(putReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("s3 put status %d", putRes.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/fe-plain-bucket/plain-object.txt", nil)
	if err := auth.SignRequest(getReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(getRes.Body)
	if getRes.StatusCode != http.StatusOK || body.String() != "plain-payload" {
		t.Fatalf("s3 get status %d body %q", getRes.StatusCode, body.String())
	}
}

func TestFieldEncryptionAPI_securityStatusEnabled(t *testing.T) {
	s := testServerWithFieldEnc(t, true)
	tok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodGet, "/api/v1/settings/security-status", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		FieldEncryption struct {
			Enabled       bool   `json:"enabled"`
			ActiveKEKID   string `json:"active_kek_id"`
			RegistryCount int    `json:"registry_count"`
		} `json:"field_encryption"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.FieldEncryption.Enabled {
		t.Fatal("expected field_encryption.enabled true")
	}
	if resp.FieldEncryption.ActiveKEKID != "kek-api-test" {
		t.Fatalf("active_kek_id %q", resp.FieldEncryption.ActiveKEKID)
	}
	if resp.FieldEncryption.RegistryCount < 1 {
		t.Fatalf("registry_count %d want >= 1", resp.FieldEncryption.RegistryCount)
	}
}

func TestFieldEncryptionAPI_accessKeyS3Roundtrip(t *testing.T) {
	s := testServerWithFieldEnc(t, true)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/fe-s3-bucket", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	keyBody, _ := json.Marshal(map[string]string{"label": "fe-s3"})
	req = authReq(http.MethodPost, "/api/v1/keys", adminTok, keyBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key %d %s", rec.Code, rec.Body.String())
	}
	var keyResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &keyResp)
	ak := keyResp["access_key"]
	sk := keyResp["secret_key"]
	if ak == "" || sk == "" {
		t.Fatalf("missing credentials: %+v", keyResp)
	}

	reporter, ok := s.Meta().(metadata.FieldEncryptionReporter)
	if !ok {
		t.Fatal("metadata store should implement FieldEncryptionReporter")
	}
	got, err := s.Meta().GetAccessKey(ak)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretKey != sk {
		t.Fatalf("decrypted secret mismatch")
	}
	if !reporter.FieldEnc().Enabled() {
		t.Fatal("field encryption should be enabled on store")
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/fe-s3-bucket/fe-object.txt", bytes.NewReader([]byte("fe-payload")))
	putReq.Header.Set("Content-Type", "text/plain")
	creds := auth.Credentials{AccessKey: ak, SecretKey: sk}
	if err := auth.SignRequest(putReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("s3 put status %d", putRes.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/fe-s3-bucket/fe-object.txt", nil)
	if err := auth.SignRequest(getReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(getRes.Body)
	if getRes.StatusCode != http.StatusOK || body.String() != "fe-payload" {
		t.Fatalf("s3 get status %d body %q", getRes.StatusCode, body.String())
	}
}
