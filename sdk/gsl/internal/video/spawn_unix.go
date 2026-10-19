//go:build !windows

package video

import (
	"os/exec"
	"syscall"
)

// detach puts the refresher in its own session (stdio already on /dev/null)
// so it outlives the status-line render and never sees its terminal signals.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
