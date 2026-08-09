//go:build !windows

package exec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// setPgid puts the child in a NEW process group whose leader is the child
// itself, so its pid doubles as the group id. Without this, the child shares
// gsl's own process group and a negative-pid kill would signal gsl (and, under
// `go test`, the test binary) — which is why the group id must be established
// at fork time and cannot be retrofitted after Start.
func setPgid(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killGroup SIGKILLs the process group led by cmd's process.
//
// kill(-pid) addresses the GROUP; kill(pid) would address only the direct
// child and leave the grandchildren holding the stdout pipe — the bug (E18).
//
// SIGKILL, not SIGTERM: these are read-only probes (`git status`, `gh pr view`,
// `claude mcp list`) with no cleanup worth waiting for, and a process that
// ignores SIGTERM is precisely the one whose deadline we are enforcing.
func killGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return os.ErrProcessDone
	}

	err := syscall.Kill(-pid, syscall.SIGKILL)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.ESRCH):
		// The whole group is already gone — the command finished on its own.
		// os/exec keys on os.ErrProcessDone to preserve the real exit status.
		return os.ErrProcessDone
	default:
		// No group to signal (e.g. Setpgid was not applied because the platform
		// refused it). Fall back to the direct child so cancellation still does
		// SOMETHING; WaitDelay remains the backstop for the leaked pipe.
		if kerr := cmd.Process.Kill(); kerr != nil {
			if errors.Is(kerr, os.ErrProcessDone) {
				return os.ErrProcessDone
			}
			return kerr
		}
		return nil
	}
}
