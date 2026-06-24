package security

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestWeakEnvVarsDevSkips(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	t.Setenv("STORAGE_JWT_SECRET", defaultJWTSecret)
	if got := WeakEnvVars(); len(got) != 0 {
		t.Fatalf("expected no weak vars in dev, got %v", got)
	}
}

func TestWeakEnvVarsDetectsJWT(t *testing.T) {
	t.Setenv("STORAGE_DEV", "")
	t.Setenv("STORAGE_JWT_SECRET", defaultJWTSecret)
	t.Setenv("STORAGE_SECRET_KEY", "custom")
	t.Setenv("STORAGE_ADMIN_PASSWORD", "custom")
	got := WeakEnvVars()
	if len(got) != 1 || got[0] != "STORAGE_JWT_SECRET" {
		t.Fatalf("unexpected weak vars: %v", got)
	}
}

func TestValidateStartupSecretsDevSkips(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	t.Setenv("STORAGE_JWT_SECRET", defaultJWTSecret)
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	ValidateStartupSecrets(logger)
	if buf.Len() != 0 {
		t.Fatalf("expected no output in dev mode, got %q", buf.String())
	}
}

func TestValidateStartupSecretsWarnsOnDefaultJWT(t *testing.T) {
	t.Setenv("STORAGE_DEV", "")
	t.Setenv("STORAGE_STRICT_SECRETS", "")
	t.Setenv("STORAGE_JWT_SECRET", defaultJWTSecret)
	t.Setenv("STORAGE_SECRET_KEY", "custom-secret")
	t.Setenv("STORAGE_ADMIN_PASSWORD", "custom-pass")
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	ValidateStartupSecrets(logger)
	if !bytes.Contains(buf.Bytes(), []byte("STORAGE_JWT_SECRET")) {
		t.Fatalf("expected JWT warning, got %q", buf.String())
	}
}
