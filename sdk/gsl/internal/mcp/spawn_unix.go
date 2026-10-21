//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// detach puts the refresher in its own session (stdio already on /dev/null)
// so it outlives the render and never sees its terminal's signals.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
