//go:build !windows

package exec

import (
	"os/exec"
	"syscall"
)

// Detach puts a refresher in its own session (stdio already on /dev/null), so
// it outlives the status-line render and never sees its terminal's signals.
// It only configures cmd (the check-deps sub-gate).
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
