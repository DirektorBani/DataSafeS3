package vaulttransit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
)

// TestVaultDevTransit runs against `vault server -dev` when VAULT_ADDR and
// VAULT_TOKEN are set. CI starts that process. Local `go test` skips it.
func TestVaultDevTransit(t *testing.T) {
	addr := os.Getenv("VAULT_ADDR")
	token := os.Getenv("VAULT_TOKEN")
	if addr == "" || token == "" {
		t.Skip("VAULT_ADDR and VAULT_TOKEN are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := enableDevTransit(ctx, addr, token); err != nil {
		t.Fatal(err)
	}
	p, err := Open(Config{
		Provider: kms.ProviderVault,
		Addr:     addr,
		Token:    token,
		Timeout:  5 * time.Second,
	})
	if err != nil || p == nil {
		t.Fatal(err)
	}
	if err := p.Test(ctx); err != nil {
		t.Fatal(err)
	}
	wrapped, ver, err := p.Wrap(ctx, "datasafe-ci-vault", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	dek, err := p.UnwrapDEK(ctx, "datasafe-ci-vault", ver, 1, wrapped)
	if err != nil || string(dek) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unwrap %q %v", dek, err)
	}
}

func enableDevTransit(ctx context.Context, addr, token string) error {
	body := bytes.NewBufferString(`{"type":"transit"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/v1/sys/mounts/transit", body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	var parsed struct {
		Errors []string `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	return &StatusError{Op: "mount", Status: resp.StatusCode, Detail: "transit mount"}
}
