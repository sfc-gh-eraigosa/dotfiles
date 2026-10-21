package mcp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RefreshEnv is set in the refresher's environment. A process that sees it is
// already a refresher and must never spawn another one.
const RefreshEnv = "GSL_MCP_REFRESH"

// ErrNoSpawn is returned when SpawnRefresh refuses to start a child: inside a
// Go test binary, inside a refresher, or on Windows. Re-exec'ing a test binary
// runs the whole suite again, detached, with nothing to reap it — the fork
// bomb internal/video learned about on 2026-10-17.
var ErrNoSpawn = errors.New("mcp: refusing to spawn refresher from a test binary or a refresher")

// SpawnRefresh starts `gsl mcp refresh` for cwd in the background — detached,
// in its own session, run from cwd (the probe's servers depend on it) — and
// does not wait. The render path calls it when Get reports RefreshDue; the
// child's Refresh is single-flight, so a race between two renders costs one
// extra short-lived process, never two probes.
func SpawnRefresh(cwd string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" || !canSpawn(self, os.Getenv, testing.Testing()) {
		return ErrNoSpawn
	}
	cmd := exec.Command(self, "mcp", "refresh") //nolint:gosec // G204: our own binary, fixed args
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), RefreshEnv+"=1")
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// canSpawn is the recursion guard, testable without forking: never from a Go
// test binary (inTest is authoritative; the name check is a second layer) and
// never from a refresher.
func canSpawn(self string, getenv func(string) string, inTest bool) bool {
	if inTest || getenv(RefreshEnv) != "" {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(self)), ".exe")
	return !strings.HasSuffix(base, ".test")
}
