package kms

import "testing"

func TestTerminalStatusRefusesMissingObjects(t *testing.T) {
	status, errs := TerminalStatus(59, 60, 0, 59)
	if status != JobCompletedWithErrors || errs != 1 {
		t.Fatalf("missing row %s %d", status, errs)
	}
	status, errs = TerminalStatus(59, 60, 0, 60)
	if status != JobCompletedWithErrors || errs != 1 {
		t.Fatalf("short done %s %d", status, errs)
	}
	status, errs = TerminalStatus(60, 60, 0, 60)
	if status != JobCompleted || errs != 0 {
		t.Fatalf("complete %s %d", status, errs)
	}
}

func TestJobCursorAndIdempotentDecide(t *testing.T) {
	items := []WorkItem{
		{Key: "a", VersionID: "1"},
		{Key: "a", VersionID: "2"},
		{Key: "b", VersionID: ""},
	}
	cursor := Cursor("a", "1")
	var next []string
	for _, it := range items {
		if AfterCursor(cursor, it.Key, it.VersionID) {
			next = append(next, Cursor(it.Key, it.VersionID))
		}
	}
	if len(next) != 2 || next[0] != Cursor("a", "2") || next[1] != Cursor("b", "") {
		t.Fatalf("resume %v", next)
	}
	atTarget := WorkItem{Key: "a", Sealed: true, Wrapped: true, KeyID: "k", KeyVersion: 2, FormatVersion: 1}
	if Decide(atTarget, JobEncryptExisting, "k", 2) != ActSkip {
		t.Fatal("rewrap of the target version must be a no-op")
	}
	old := atTarget
	old.KeyVersion = 1
	if Decide(old, JobEncryptExisting, "k", 2) != ActRewrap {
		t.Fatal("older version should rewrap")
	}
	plain := WorkItem{Key: "p"}
	if Decide(plain, JobEncryptExisting, "k", 2) != ActEncrypt {
		t.Fatal("plaintext should be encrypted")
	}
	shredded := WorkItem{Key: "a", KeyID: "k", Sealed: true}
	if Decide(shredded, JobCryptoShred, "k", 2) != ActSkip {
		t.Fatal("missing wrapped DEK is already shredded")
	}
	live := shredded
	live.Wrapped = true
	if Decide(live, JobCryptoShred, "k", 2) != ActShred {
		t.Fatal("wrapped DEK should be removed")
	}
}
