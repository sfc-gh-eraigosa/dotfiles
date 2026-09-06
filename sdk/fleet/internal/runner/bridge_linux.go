//go:build linux

package runner

import (
	"os/exec"
	"syscall"
)

// setDeathSignal asks the kernel to SIGTERM the bridge child if fleet dies
// without cleaning up — the one case a context cannot cover. A bridge never
// outlives fleet on Linux even under SIGKILL.
func setDeathSignal(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
