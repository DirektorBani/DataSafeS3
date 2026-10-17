package main

import (
	"errors"
	"os"
)

// errAgentBusy means another agent process already holds the data directory.
// The transfer journal is one bolt file, so a second process must not open it.
var errAgentBusy = errors.New("another agent is already using this data directory")

func releaseAgentLock(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}
