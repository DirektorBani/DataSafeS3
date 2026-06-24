package syncapp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizePrefix(t *testing.T) {
	if got := NormalizePrefix("reports"); got != "reports/" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizePrefix("/"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRelativeKey(t *testing.T) {
	rel, ok := relativeKey("reports/q1.pdf", "reports/")
	if !ok || rel != "q1.pdf" {
		t.Fatalf("got %q %v", rel, ok)
	}
}

func TestParseConflictPolicy(t *testing.T) {
	if ParseConflictPolicy("keep_both") != ConflictKeepBoth {
		t.Fatal()
	}
	if ParseConflictPolicy("") != ConflictLastWriteWins {
		t.Fatal()
	}
}

func TestIsConflict(t *testing.T) {
	last := time.Now().Add(-time.Hour)
	prev := FileState{ETag: "aaa", LastSyncedAt: last}
	remote := ObjectItem{ETag: `"bbb"`, LastModified: time.Now()}
	if !isConflict("ccc", time.Now(), remote, prev) {
		t.Fatal("expected conflict")
	}
	if isConflict("bbb", time.Now(), remote, prev) {
		t.Fatal("etag match should not conflict")
	}
}

func TestStateSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	st := &State{Files: map[string]FileState{"a.txt": {Size: 1}}}
	if err := st.Save("test"); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState("test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Files["a.txt"].Size != 1 {
		t.Fatalf("state lost: %+v", loaded.Files)
	}
}

func TestListConflictsEmpty(t *testing.T) {
	dir := t.TempDir()
	names, err := ListConflicts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty, got %v", names)
	}
}

func TestConflictBackupFlat(t *testing.T) {
	dir := t.TempDir()
	p := conflictBackupPathFlat(dir, "docs/report.pdf", time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := ListConflicts(dir)
	if err != nil || len(names) != 1 {
		t.Fatalf("names=%v err=%v", names, err)
	}
}

func TestRunOncePush(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RunOnce(Options{Folder: local, Bucket: "files", Push: true})
	if err == nil {
		t.Fatal("expected error without client")
	}
}
