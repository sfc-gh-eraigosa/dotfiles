package video

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"

	gslexec "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/exec"
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
	gslexec.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// canSpawn is the shared recursion guard (internal/exec.CanSpawn) keyed on
// this refresher's RefreshEnv.
func canSpawn(self string, getenv func(string) string, inTest bool) bool {
	return gslexec.CanSpawn(self, getenv, inTest, RefreshEnv)
}
