//go:build vault

package security_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration plan (build tag vault, env TEST_VAULT_ADDR):
//  1. Vault reachable and unsealed (or dev mode)
//  2. KV v2 path secret/datasafe/bootstrap contains jwt_secret, s3_secret_key, admin_password
//  3. Optional: STORAGE_URL /healthz when stack is up
//  4. Optional: security-status weak_secrets empty when STORAGE_DEV=false and secrets injected
//
// Run:
//
//	go test -tags=vault ./internal/security/ -run VaultInjection -v
//
// Skip when TEST_VAULT_ADDR is unset (default local dev / CI without Vault profile).
func TestVaultInjection_KVSecretsPresent(t *testing.T) {
	addr := strings.TrimSuffix(os.Getenv("TEST_VAULT_ADDR"), "/")
	if addr == "" {
		t.Skip("set TEST_VAULT_ADDR (e.g. http://127.0.0.1:8200) with compose --profile vault")
	}
	token := os.Getenv("TEST_VAULT_TOKEN")
	if token == "" {
		token = "root"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, addr+"/v1/sys/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("vault health: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != 429 {
		t.Fatalf("vault health status %d", res.StatusCode)
	}
	var health struct {
		Sealed      bool `json:"sealed"`
		Initialized bool `json:"initialized"`
	}
	_ = json.NewDecoder(res.Body).Decode(&health)
	if health.Sealed {
		t.Fatal("vault is sealed")
	}

	kvPath := os.Getenv("VAULT_KV_PATH")
	if kvPath == "" {
		kvPath = "secret/data/datasafe/bootstrap"
	} else if !strings.Contains(kvPath, "/data/") {
		kvPath = "secret/data/" + strings.TrimPrefix(kvPath, "secret/")
	}
	req, err = http.NewRequest(http.MethodGet, addr+"/v1/"+kvPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Vault-Token", token)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("kv get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("kv get status %d: %s", res.StatusCode, body)
	}
	var payload struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"jwt_secret", "s3_secret_key", "admin_password"} {
		if payload.Data.Data[key] == "" {
			t.Fatalf("missing %s in %s", key, kvPath)
		}
	}
}

func TestVaultInjection_StorageHealthAndSecurityStatus(t *testing.T) {
	if os.Getenv("VAULT_PROFILE") != "1" {
		t.Skip("set VAULT_PROFILE=1 when storage-server is started with docker-compose.vault.yml")
	}
	base := strings.TrimSuffix(os.Getenv("STORAGE_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:9000"
	}
	adminUser := envOr("VAULT_TEST_ADMIN_USER", "admin")
	adminPass := os.Getenv("VAULT_TEST_ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = "VaultIntegAdmin9!"
	}

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz status %d", res.StatusCode)
	}

	loginBody := fmt.Sprintf(`{"username":%q,"password":%q}`, adminUser, adminPass)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/admin/login", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("login status %d: %s", res.StatusCode, body)
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tok); err != nil {
		t.Fatal(err)
	}
	if tok.Token == "" {
		t.Fatal("empty token")
	}

	req, err = http.NewRequest(http.MethodGet, base+"/api/v1/settings/security-status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("security-status %d", res.StatusCode)
	}
	var status struct {
		WeakSecrets []string `json:"weak_secrets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if len(status.WeakSecrets) > 0 {
		t.Fatalf("weak_secrets=%v", status.WeakSecrets)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
