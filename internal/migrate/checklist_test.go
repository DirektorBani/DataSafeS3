package migrate

import (
	"strings"
	"testing"
)

func TestChecklistMarkdown(t *testing.T) {
	md := ChecklistMarkdown()
	for _, want := range []string{
		"MinIO → DataSafeS3",
		"Freeze or pause writers",
		"minio-cutover-smoke.ps1",
		"Object Lock",
		"100% MinIO API parity",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("checklist missing %q", want)
		}
	}
	if !strings.HasPrefix(md, "# ") {
		t.Fatal("expected markdown heading")
	}
}
