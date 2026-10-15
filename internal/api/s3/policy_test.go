package s3_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/observability"
)

func TestPolicyDeniesUnauthorizedGet(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	setupBucket(t, ts, creds, "locked")
	token, err := srv.AdminToken()
	if err != nil {
		t.Fatal(err)
	}
	policy := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":"*",
			"Action":["s3:PutObject"],
			"Resource":["arn:aws:s3:::locked/*"]
		}]
	}`
	payload, _ := json.Marshal(map[string]string{"policy": policy})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/buckets/locked/policy", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set policy status %d", resp.StatusCode)
	}

	putBody := bytes.NewReader([]byte("secret"))
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/locked/secret.txt", putBody)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	createKeyReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users", strings.NewReader(`{"username":"otheruser","password":"pass123","role":"user","email":"o@test.com"}`))
	createKeyReq.Header.Set("Content-Type", "application/json")
	createKeyReq.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(createKeyReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user %d", resp.StatusCode)
	}
	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/login", strings.NewReader(`{"username":"otheruser","password":"pass123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	loginBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginBody, &loginResp); err != nil {
		t.Fatal(err)
	}

	createKeyReq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/keys", strings.NewReader(`{"label":"other"}`))
	createKeyReq.Header.Set("Content-Type", "application/json")
	createKeyReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	resp, err = http.DefaultClient.Do(createKeyReq)
	if err != nil {
		t.Fatal(err)
	}
	keyBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key %d: %s", resp.StatusCode, keyBody)
	}

	var keyResp struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(keyBody, &keyResp); err != nil {
		t.Fatal(err)
	}
	other := auth.Credentials{AccessKey: keyResp.AccessKey, SecretKey: keyResp.SecretKey}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/locked/secret.txt", nil)
	signRequest(t, req, other, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for denied get, got %d", resp.StatusCode)
	}
}

func TestListAllMyBucketsRequiresAllow(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	srv, owner := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	setupBucket(t, ts, owner, "listed")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, owner, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner list %d", resp.StatusCode)
	}

	token, err := srv.AdminToken()
	if err != nil {
		t.Fatal(err)
	}
	create, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users", strings.NewReader(`{"username":"plain","password":"pass123","role":"user","email":"p@test.com"}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user %d", resp.StatusCode)
	}
	login, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/login", strings.NewReader(`{"username":"plain","password":"pass123"}`))
	login.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	loginBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginBody, &session); err != nil || session.Token == "" {
		t.Fatalf("login %s %v", loginBody, err)
	}
	keyReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/keys", strings.NewReader(`{"label":"plain"}`))
	keyReq.Header.Set("Content-Type", "application/json")
	keyReq.Header.Set("Authorization", "Bearer "+session.Token)
	resp, err = http.DefaultClient.Do(keyReq)
	if err != nil {
		t.Fatal(err)
	}
	keyBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("key %d %s", resp.StatusCode, keyBody)
	}
	var keyResp struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(keyBody, &keyResp); err != nil {
		t.Fatal(err)
	}
	plain := auth.Credentials{AccessKey: keyResp.AccessKey, SecretKey: keyResp.SecretKey}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, plain, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "AccessDenied") {
		t.Fatalf("key with nothing owned %d %s", resp.StatusCode, body)
	}

	narrow := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::datasafe:access-key/%s"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::listed/*"}]}`, keyResp.AccessKey)
	payload, _ := json.Marshal(map[string]string{"policy": narrow})
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/buckets/listed/policy", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("narrow policy %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, plain, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "AccessDenied") {
		t.Fatalf("get-only policy without ownership %d %s", resp.StatusCode, body)
	}

	mine, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/buckets/mine", strings.NewReader(`{}`))
	mine.Header.Set("Content-Type", "application/json")
	mine.Header.Set("Authorization", "Bearer "+session.Token)
	resp, err = http.DefaultClient.Do(mine)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create mine %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, plain, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "mine") || !strings.Contains(string(body), "listed") {
		t.Fatalf("owner plus policy-visible bucket %d %s", resp.StatusCode, body)
	}

	adminLogin, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	adminLogin.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(adminLogin)
	if err != nil {
		t.Fatal(err)
	}
	adminBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var adminSession struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(adminBody, &adminSession); err != nil || adminSession.Token == "" {
		t.Fatalf("admin login %s", adminBody)
	}
	adminKey, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/keys", strings.NewReader(`{"label":"admin"}`))
	adminKey.Header.Set("Content-Type", "application/json")
	adminKey.Header.Set("Authorization", "Bearer "+adminSession.Token)
	resp, err = http.DefaultClient.Do(adminKey)
	if err != nil {
		t.Fatal(err)
	}
	adminKeyBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin key %d %s", resp.StatusCode, adminKeyBody)
	}
	if err := json.Unmarshal(adminKeyBody, &keyResp); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, auth.Credentials{AccessKey: keyResp.AccessKey, SecretKey: keyResp.SecretKey}, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin list %d", resp.StatusCode)
	}
}

func TestAdminLoginAndMetrics(t *testing.T) {
	srv, _ := testServer(t)
	handler := observability.MetricsMiddleware(srv.Handler())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	loginBody := `{"username":"admin","password":"admin"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login status %d: %s", resp.StatusCode, body)
	}

	resp, err = http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status %d", resp.StatusCode)
	}
	metricsBody, _ := io.ReadAll(resp.Body)
	body := string(metricsBody)
	if !strings.Contains(body, "datasafe_http_requests_total") {
		t.Fatalf("expected datasafe_http_requests_total in metrics")
	}
	if !strings.Contains(body, "datasafe_buckets_total") {
		t.Fatalf("expected datasafe_buckets_total in metrics")
	}
}
