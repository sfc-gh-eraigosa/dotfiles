package exec

import (
	"errors"
	"os/exec"
)

// IsExpectedExit reports whether err is a ROUTINE non-zero exit from a command
// that ran to completion and simply answered "no": `git status` outside a repo,
// `gh pr view` on a branch with no PR, `claude mcp list` with no servers.
//
// It exists for one reason: log level. Those exits are the NORMAL case — a
// session produced ~1.6k of them — and logging them at Warn buried the records
// that actually matter (segment.panic) in noise. They belong at Debug.
//
// It is deliberately FALSE for everything that is not routine, so those keep
// their Warn:
//
//   - a process we SIGKILLed on cancellation (ProcessState.ExitCode() == -1 for
//     a signal death, so the ExitCode() > 0 test excludes it);
//   - exec.ErrWaitDelay — the command exited but leaked an orphan holding the
//     pipes, which is exactly the containment failure we are hunting;
//   - a context deadline / cancellation;
//   - a missing binary (*exec.Error), which is a real misconfiguration.
func IsExpectedExit(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	// ExitCode() is -1 when the process was terminated by a signal or has not
	// exited — neither is a routine "the command answered no".
	return ee.ExitCode() > 0
}
