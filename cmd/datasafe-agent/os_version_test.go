package main

import "testing"

func TestWindows11UsesBuildNumber(t *testing.T) {
	got := formatWindowsVersion(10, 0, 26200, "amd64")
	if got != "Windows 11 (10.0.26200) amd64" {
		t.Fatal(got)
	}
	got = formatWindowsVersion(10, 0, 19045, "amd64")
	if got != "Windows 10 (10.0.19045) amd64" {
		t.Fatal(got)
	}
	got = formatWindowsVersion(10, 0, 22000, "")
	if got != "Windows 11 (10.0.22000)" {
		t.Fatal(got)
	}
}

func TestChooseCommitPrefersCIFlag(t *testing.T) {
	if got := chooseCommit("abc123def456", "ffffffffffffffffffffffffffffffffffffffff"); got != "abc123def456" {
		t.Fatal(got)
	}
	if got := chooseCommit("unknown", "abcdef1234567890dead"); got != "abcdef123456" {
		t.Fatal(got)
	}
	if got := chooseCommit("", ""); got != "unknown" {
		t.Fatal(got)
	}
}
