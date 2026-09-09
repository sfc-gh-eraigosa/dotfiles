package winhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// FallbackPaths are tried when powershell.exe is not on PATH.
//
// This is not paranoia: WSL's interop PATH entries can be missing on an
// otherwise perfectly healthy system, so a `command -v powershell.exe` miss
// does not mean Windows is unreachable. Looking here first turns a spurious
// "not on WSL" into a working query.
var FallbackPaths = []string{
	"/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
	"/mnt/c/Windows/system32/WindowsPowerShell/v1.0/powershell.exe",
}

// DefaultTimeout bounds a single interop call. Interop is normally
// sub-second; a hang here would otherwise stall an install.
const DefaultTimeout = 20 * time.Second

// PowerShell runs queries against the real Windows host.
type PowerShell struct {
	// Path to powershell.exe. Resolved by NewPowerShell.
	Path string
	// Timeout for a single call; zero means DefaultTimeout.
	Timeout time.Duration
}

// ErrNoPowerShell reports that Windows is unreachable from here. Callers treat
// this as "not usable on this host" and decline safely — never as a failure.
var ErrNoPowerShell = fmt.Errorf("powershell.exe not found (PATH or %v)", FallbackPaths)

// NewPowerShell locates powershell.exe, preferring PATH and falling back to the
// well-known absolute locations.
func NewPowerShell() (*PowerShell, error) {
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return &PowerShell{Path: p}, nil
	}
	for _, p := range FallbackPaths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return &PowerShell{Path: p}, nil
		}
	}
	return nil, ErrNoPowerShell
}

// Run executes one query. -NonInteractive and -NoProfile keep a user's profile
// from injecting output into what we parse, which would corrupt the JSON.
func (p *PowerShell) Run(ctx context.Context, s script) ([]byte, error) {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.Path, "-NoProfile", "-NonInteractive", "-Command", s.source)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("powershell %s timed out after %s", s.kind, timeout)
		}
		return nil, fmt.Errorf("powershell %s: %w", s.kind, err)
	}
	return out, nil
}
