package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunMigrateChecklist(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	err = runMigrate([]string{"checklist", "minio"})
	_ = w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	for _, want := range []string{"MinIO → DataSafeS3", "Cutover", "minio-cutover-smoke.ps1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("checklist output missing %q", want)
		}
	}
}

func TestRunMigrateChecklistUnsupported(t *testing.T) {
	err := runMigrate([]string{"checklist", "ceph"})
	if err == nil {
		t.Fatal("expected error for unsupported target")
	}
}

func TestRunMigrateUnknownSubcommand(t *testing.T) {
	err := runMigrate([]string{"import"})
	if err == nil {
		t.Fatal("expected error")
	}
}
