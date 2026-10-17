//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func acquireAgentLock(dataDir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dataDir, "agent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&windows.Overlapped{},
	)
	if err == nil {
		return f, nil
	}
	_ = f.Close()
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, errAgentBusy
	}
	return nil, err
}
