//go:build !linux && !windows

package main

import (
	"fmt"
	"os"
)

func acquireAgentLock(string) (*os.File, error) {
	return nil, fmt.Errorf("agent lock is only implemented on windows and linux")
}
