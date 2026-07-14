package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecklistMarkdown_RequiredSections(t *testing.T) {
	md := ChecklistMarkdown()
	sections := []string{
		"## Before sync",
		"## Initial sync",
		"## Cutover",
		"## After cutover",
		"## Out of scope (manual)",
	}
	for _, s := range sections {
		if !strings.Contains(md, s) {
			t.Errorf("missing section %q", s)
		}
	}
}

func TestChecklistMarkdown_HonestyAndSafety(t *testing.T) {
	md := ChecklistMarkdown()
	checks := []struct {
		name string
		want string
	}{
		{"healthz", "GET /healthz"},
		{"freeze", "Freeze or pause writers"},
		{"smoke", "minio-cutover-smoke.ps1"},
		{"object lock re-enable", "Object Lock"},
		{"iam remap", "Remap IAM"},
		{"rollback window", "rollback"},
		{"no auto IAM import", "MinIO IAM users/groups import"},
		{"no parity claim", "100% MinIO API parity"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q", c.name, c.want)
		}
	}
}

func TestChecklistMarkdown_CheckboxDensity(t *testing.T) {
	md := ChecklistMarkdown()
	n := strings.Count(md, "- [ ]")
	if n < 12 {
		t.Fatalf("expected >=12 checklist items, got %d", n)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found from test cwd")
	return ""
}

func TestMigrationKitArtifactsExist(t *testing.T) {
	root := findRepoRoot(t)
	required := []string{
		filepath.Join("docs", "operations-guide", "en", "migrate-from-minio.md"),
		filepath.Join("docs", "operations-guide", "ru", "migrate-from-minio.md"),
		filepath.Join("docs", "operations-guide", "en", "examples", "rclone-minio-to-datasafe.conf"),
		filepath.Join("scripts", "migrate", "minio-cutover-smoke.ps1"),
		filepath.Join("scripts", "migrate", "README.md"),
		filepath.Join("docs", "architecture", "adr", "0001-migration-kit.md"),
		filepath.Join("docs", "architecture", "adr", "0002-event-sink-interface.md"),
		filepath.Join("docs", "architecture", "adr", "0003-immutable-backup-path.md"),
		filepath.Join("docs", "architecture", "adr", "0004-inventory-jobs.md"),
		filepath.Join("docs", "architecture", "adr", "0005-scripted-ha-promote.md"),
		filepath.Join("internal", "events", "sink.go"),
		filepath.Join("internal", "inventory", "types.go"),
		filepath.Join("internal", "ha", "promote", "doc.go"),
	}
	for _, rel := range required {
		p := filepath.Join(root, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing artifact %s: %v", rel, err)
		}
	}
}

func TestMigrateFromMinioGuideHonesty(t *testing.T) {
	root := findRepoRoot(t)

	en, err := os.ReadFile(filepath.Join(root, "docs", "operations-guide", "en", "migrate-from-minio.md"))
	if err != nil {
		t.Fatal(err)
	}
	enText := string(en)
	for _, want := range []string{"not a drop-in", "rclone", "Honesty", "IAM"} {
		if !strings.Contains(enText, want) {
			t.Errorf("EN guide missing %q", want)
		}
	}

	ru, err := os.ReadFile(filepath.Join(root, "docs", "operations-guide", "ru", "migrate-from-minio.md"))
	if err != nil {
		t.Fatal(err)
	}
	ruText := string(ru)
	for _, want := range []string{"Честно", "rclone", "импортируются"} {
		if !strings.Contains(ruText, want) {
			t.Errorf("RU guide missing %q", want)
		}
	}
}

func TestChangelogHas111MigrationKit(t *testing.T) {
	root := findRepoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"## [1.1.1]", "MinIO migration kit", "scripts/migrate"} {
		if !strings.Contains(text, want) {
			t.Errorf("CHANGELOG missing %q", want)
		}
	}
}
