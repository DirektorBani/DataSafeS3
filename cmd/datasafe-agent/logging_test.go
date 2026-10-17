package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsInLogs(t *testing.T) {
	dir := t.TempDir()
	logger, err := openLogger(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	const (
		password  = "s3cret-password"
		totp      = "918273"
		accessKey = "AKIASECRET"
		signature = "deadbeefsignature"
	)
	logger.Info("login password=" + password)
	logger.Info("mfa X-DataSafe-MFA-Token: " + totp)
	logger.Info("mfa_code=" + totp)
	logger.Info("step_up_code=" + totp)
	logger.Info("presign https://storage.example/bucket/key?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=" + accessKey + "&X-Amz-Signature=" + signature)
	logger.Info(`json {"password":"` + password + `","token":"jwt-secret-token","mfa_code":"` + totp + `"}`)
	logger.Info("Authorization: Bearer jwt-secret-token")
	logger.Sync()

	body := readLogs(t, logger.dir)
	for _, secret := range []string{password, totp, accessKey, signature, "jwt-secret-token"} {
		if strings.Contains(body, secret) {
			t.Fatalf("secret %q leaked into the log:\n%s", secret, body)
		}
	}
	if !strings.Contains(body, "login") || !strings.Contains(body, "[redacted]") {
		t.Fatalf("log did not keep the event: %s", body)
	}

	extra := filepath.Join(logger.dir, "profiles.json")
	if err := os.WriteFile(extra, []byte(password), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{dataDir: dir, log: logger}
	dest := t.TempDir()
	result, err := app.diagExport(map[string]any{"dest_path": dest})
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := result.(map[string]any)
	path, _ := saved["path"].(string)
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var archived strings.Builder
	sawVersions := false
	for _, file := range zr.File {
		if file.Name == "profiles.json" || strings.Contains(file.Name, "transfers") {
			t.Fatalf("archive includes %s", file.Name)
		}
		if file.Name == "versions.txt" {
			sawVersions = true
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		archived.Write(buf)
	}
	if !sawVersions {
		t.Fatal("archive has no versions.txt")
	}
	text := archived.String()
	for _, secret := range []string{password, totp, accessKey, signature, "jwt-secret-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret %q leaked into the archive", secret)
		}
	}
	for _, part := range []string{"DataSafeS3 Desktop", DesktopVersion, "commit:", "os:", "server: none"} {
		if !strings.Contains(text, part) {
			t.Fatalf("versions missing %q: %s", part, text)
		}
	}
}

func TestVersionsTextNamesCommitOSAndServer(t *testing.T) {
	prev := DesktopCommit
	DesktopCommit = "abc123def456"
	t.Cleanup(func() { DesktopCommit = prev })
	text := versionsText("1.6.1")
	for _, part := range []string{"commit: abc123def456", "server: 1.6.1", "os:", "version: " + DesktopVersion} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %s", part, text)
		}
	}
}

func TestLogRotation(t *testing.T) {
	logger, err := openLogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	logger.maxBytes = 80
	logger.maxFiles = 3
	for i := 0; i < 30; i++ {
		logger.Info("rotation line")
	}
	logger.Sync()
	entries, err := os.ReadDir(logger.dir)
	if err != nil {
		t.Fatal(err)
	}
	var logs int
	for _, ent := range entries {
		if isLogName(ent.Name()) {
			logs++
		}
	}
	if logs < 2 || logs > 3 {
		t.Fatalf("log files %d", logs)
	}
}

func readLogs(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, ent := range entries {
		if !isLogName(ent.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
	}
	return b.String()
}
