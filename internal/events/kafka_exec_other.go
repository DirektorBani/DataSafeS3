//go:build datasafe_sinkexec && !linux

package events

import "os/exec"

func dieWithParent(cmd *exec.Cmd) {}
