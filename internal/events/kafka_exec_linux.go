//go:build datasafe_sinkexec && linux

package events

import (
	"os/exec"
	"syscall"
)

// dieWithParent asks the kernel to signal the child when this process dies,
// including when it is killed and does not run Close.
func dieWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
