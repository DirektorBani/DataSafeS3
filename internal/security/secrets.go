package security

import (
	"log/slog"
	"os"
)

const (
	defaultJWTSecret  = "datasafe-jwt-secret"
	defaultSecretKey  = "datasafesecret"
	defaultAdminPass  = "admin"
	secretsDocPath    = "docs/operations-guide/en/backup-restore.md#sse-master-key-rotation"
	jwtSecretsDocPath = "docs/getting-started/en/onboarding.md#security-hardening"
)

// JWTSecretsDocPath returns the documentation path for JWT hardening.
func JWTSecretsDocPath() string {
	return jwtSecretsDocPath
}

// ValidateStartupSecrets logs warnings or exits when default credentials are used in non-dev mode.
func ValidateStartupSecrets(logger *slog.Logger) {
	if isDevMode() {
		return
	}
	strict := os.Getenv("STORAGE_STRICT_SECRETS") == "true"
	weak := WeakEnvVars()
	if len(weak) == 0 {
		return
	}
	msg := "production security: rotate default secrets before go-live"
	logger.Warn(msg,
		"weak_env", weak,
		"jwt_doc", jwtSecretsDocPath,
		"sse_doc", secretsDocPath,
		"strict_mode", "set STORAGE_STRICT_SECRETS=true to fail fast",
	)
	if strict {
		logger.Error("STORAGE_STRICT_SECRETS=true and default secrets detected; refusing to start")
		os.Exit(1)
	}
}

func isDevMode() bool {
	switch os.Getenv("STORAGE_DEV") {
	case "1", "true", "yes":
		return true
	}
	return false
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// WeakEnvVars returns environment variable names still using insecure defaults (empty in dev mode).
func WeakEnvVars() []string {
	if isDevMode() {
		return nil
	}
	jwt := envOr("STORAGE_JWT_SECRET", defaultJWTSecret)
	secretKey := envOr("STORAGE_SECRET_KEY", defaultSecretKey)
	adminPass := envOr("STORAGE_ADMIN_PASSWORD", defaultAdminPass)

	var weak []string
	if jwt == "" || jwt == defaultJWTSecret {
		weak = append(weak, "STORAGE_JWT_SECRET")
	}
	if secretKey == defaultSecretKey {
		weak = append(weak, "STORAGE_SECRET_KEY")
	}
	if adminPass == defaultAdminPass {
		weak = append(weak, "STORAGE_ADMIN_PASSWORD")
	}
	return weak
}
