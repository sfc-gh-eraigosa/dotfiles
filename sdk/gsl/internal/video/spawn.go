package video

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
const RefreshEnv = "GSL_VIDEO_REFRESH"

// ErrNoSpawn is returned when SpawnRefresher refuses to start a child: inside
// a Go test binary, or inside a refresher. Re-exec'ing a test binary runs the
// whole test suite again, which renders the video segment, which spawns again,
// detached, so nothing reaps it: a fork bomb (2026-10-17, ~2,800 cmd.test
// processes took the WSL VM down).
var ErrNoSpawn = errors.New("video: refusing to spawn refresher from a test binary or a refresher")

// SpawnRefresher re-execs this binary as `gsl video-refresh`, detached, and
// does not wait for it. The child inherits lock (as fd 3), so the lock stays
// held until the refresher exits. It refuses (ErrNoSpawn) inside any Go test
// binary, however it is named, when RefreshEnv is already set, and on Windows.
func SpawnRefresher(lock *os.File) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" || !canSpawn(self, os.Getenv, testing.Testing()) {
		return ErrNoSpawn
	}
	cmd := exec.Command(self, "video-refresh")
	cmd.Env = append(os.Environ(), RefreshEnv+"=1")
	if lock != nil {
		cmd.ExtraFiles = []*os.File{lock}
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// canSpawn is the recursion guard, split out so it is testable without
// forking. self is the running executable; getenv is os.Getenv and inTest is
// testing.Testing() in production. inTest is the authoritative check: a test
// binary built with `go test -c -o <name>` has no .test suffix, and Go stops
// parsing flags at the first non-flag argument, so re-exec'ing it as
// `<bin> video-refresh` runs the entire suite. The name check stays as a
// second layer.
func canSpawn(self string, getenv func(string) string, inTest bool) bool {
	if inTest || getenv(RefreshEnv) != "" {
		return false
	}
	base := strings.ToLower(filepath.Base(self))
	base = strings.TrimSuffix(base, ".exe")
	return !strings.HasSuffix(base, ".test")
}
