package video

import (
	"bytes"
	"context"
	"os/exec"

	gslexec "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/exec"
)

// Runner is the subprocess seam for GPU probes (nvidia-smi, powershell.exe),
// the fourth os/exec seam next to git, gh and mcp.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemRunner shells out to real binaries on $PATH. Stdout only: stderr noise
// (PowerShell CLIXML progress) must not reach the parsers.
type SystemRunner struct{}

// Run implements Runner with the shared containment (own process group,
// group kill on cancel, bounded WaitDelay).
func (SystemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	gslexec.Harden(cmd)
	if err := cmd.Run(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
