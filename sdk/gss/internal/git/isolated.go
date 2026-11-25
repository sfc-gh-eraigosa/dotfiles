package git

import "os/exec"

// RunIsolated runs the binary at bin with exactly env as its environment
// and dir as its working directory, and returns its exit error. Nothing is
// inherited from gss's own environment. It is the seam for the one tool gss
// asks that is not git or gh: gff, for the approval-token flag (#472), which
// must not see a PATH, HOME or XDG_* a command set for gss.
func RunIsolated(bin, dir string, env []string, args ...string) error {
	cmd := exec.Command(bin, args...)
	if env == nil {
		// a nil Env would mean "inherit gss's own"
		env = []string{}
	}
	cmd.Env = env
	cmd.Dir = dir
	return cmd.Run()
}
