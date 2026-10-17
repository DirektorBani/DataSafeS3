package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestAgentLockRejectsASecondHolder(t *testing.T) {
	if os.Getenv("DATASAFE_LOCK_PROBE") == "1" {
		held, err := acquireAgentLock(os.Getenv("DATASAFE_LOCK_DIR"))
		if err != nil {
			os.Exit(2)
		}
		releaseAgentLock(held)
		os.Exit(0)
	}
	dir := t.TempDir()
	held, err := acquireAgentLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := probeAgentLock(dir); err == nil {
		releaseAgentLock(held)
		t.Fatal("second process acquired the lock")
	}
	releaseAgentLock(held)
	if err := probeAgentLock(dir); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
}

func probeAgentLock(dir string) error {
	cmd := exec.Command(os.Args[0], "-test.run=^TestAgentLockRejectsASecondHolder$")
	cmd.Env = append(os.Environ(), "DATASAFE_LOCK_PROBE=1", "DATASAFE_LOCK_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w %s", err, out)
	}
	return nil
}
