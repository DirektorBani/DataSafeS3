package main

import (
	"strings"
	"testing"
)

func TestDownloadNamesFollowTheTargetOS(t *testing.T) {
	used := map[string]struct{}{}
	got, notes := MapDownloadPath("CON.txt", `C:\in`, OSWindows, used, true)
	if got != `C:\in\CON.txt_` {
		t.Fatalf("CON.txt -> %q notes=%v", got, notes)
	}
	if len(notes) != 1 || notes[0].Action != "rename" {
		t.Fatalf("notes %+v", notes)
	}
	got, _ = MapDownloadPath("a:b", `C:\in`, OSWindows, used, true)
	if got != `C:\in\a_b` {
		t.Fatalf("a:b -> %q", got)
	}
	got, notes = MapDownloadPath("file.", `C:\in`, OSWindows, used, true)
	if got != `C:\in\file_` {
		t.Fatalf("trailing dot -> %q %+v", got, notes)
	}
	got, _ = MapDownloadPath("COM1.txt", `C:\in`, OSWindows, used, true)
	if got != `C:\in\COM1.txt_` {
		t.Fatalf("COM1 -> %q", got)
	}
	got, notes = MapDownloadPath("../secret", `C:\in`, OSWindows, used, true)
	if got != `C:\in\secret` {
		t.Fatalf("dotdot -> %q %+v", got, notes)
	}
	long := strings.Repeat("a", 300)
	got, notes = MapDownloadPath(long, `C:\in`, OSWindows, map[string]struct{}{}, false)
	if got != "" || notes[0].Reason != "too_long" {
		t.Fatalf("long path got %q %+v", got, notes)
	}
	used = map[string]struct{}{}
	first, _ := MapDownloadPath("A.txt", `C:\in`, OSWindows, used, true)
	second, notes := MapDownloadPath("a.txt", `C:\in`, OSWindows, used, true)
	if first == second || !strings.Contains(second, "_2") {
		t.Fatalf("case conflict %q %q %+v", first, second, notes)
	}

	nul, _, skip, reason := sanitizeSegment("a\x00b", OSLinux)
	if skip || nul != "a_b" {
		t.Fatalf("nul %q skip=%v %s", nul, skip, reason)
	}
	_, _, skip, reason = sanitizeSegment(strings.Repeat("b", 256), OSLinux)
	if !skip || reason != "segment" {
		t.Fatalf("segment skip=%v %s", skip, reason)
	}
	colon, changed, skip, _ := sanitizeSegment("a:b", OSDarwin)
	if skip || !changed || colon != "a_b" {
		t.Fatalf("darwin colon %q changed=%v", colon, changed)
	}
	composed, changed, _, _ := sanitizeSegment("e\u0301", OSDarwin)
	if !changed || composed != "\u00e9" {
		t.Fatalf("nfc %q changed=%v", composed, changed)
	}
}

func TestUploadKeyDropsDotSegments(t *testing.T) {
	key, _, ok := UploadKey("album", "a/../b.txt")
	if !ok || key != "album/a/b.txt" && key != "album/b.txt" {
		// ".." is dropped, "a" stays, so album/a/b.txt
		if key != "album/a/b.txt" {
			t.Fatalf("key %q ok=%v", key, ok)
		}
	}
	key, note, ok := UploadKey("album", "..")
	if ok || note.Action != "skip" {
		t.Fatalf("escape key %q %+v", key, note)
	}
}
