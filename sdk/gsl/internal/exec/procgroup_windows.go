//go:build windows

package exec

import (
	"os"
	"os/exec"
)

// setPgid is a NO-OP on Windows.
//
// Windows has no POSIX process groups and no Setpgid in SysProcAttr; the
// equivalent containment primitive is a Job Object, which is a materially
// different design (create the job, assign the process, terminate the job) and
// is out of scope for this objective — gsl's supported hosts are WSL2, Linux,
// and macOS. WaitDelay, which IS portable, still bounds the pipe wait here, so
// the deadline guarantee (E18) holds on Windows even without group semantics.
func setPgid(_ *exec.Cmd) {}

// killGroup degrades to killing the direct child on Windows. See setPgid.
func killGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
