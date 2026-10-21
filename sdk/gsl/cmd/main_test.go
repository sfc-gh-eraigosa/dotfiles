package cmd

import (
	"os"
	"sync/atomic"
	"testing"
)

// videoSpawnCalls counts how often a status-line render asked for a GPU
// refresher during this test run. The real spawner is never used here.
var videoSpawnCalls atomic.Int64

// TestMain replaces the video refresher with a no-op for the whole package.
// runStatusLine wires the real one, and from inside `go test` that would
// re-exec cmd.test, which runs this suite again, which spawns again: the
// 2026-10-17 fork bomb. video.SpawnRefresher also refuses on its own now;
// this keeps the tests from even asking.
//
// It also points XDG_STATE_HOME at a private directory: the video segment
// reads $XDG_STATE_HOME/gsl/video.json, and a real reading left on the host
// (any `gsl status` run) made segment-count tests depend on the machine.
func TestMain(m *testing.M) {
	videoSpawn = func(*os.File) error {
		videoSpawnCalls.Add(1)
		return nil
	}
	mcpSpawn = func(string) error { return nil } // never re-exec cmd.test
	state, err := os.MkdirTemp("", "gsl-cmd-test-state-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_STATE_HOME", state)
	code := m.Run()
	_ = os.RemoveAll(state)
	os.Exit(code)
}

func TestVideoSpawn_IsNoOpUnderTest(t *testing.T) {
	before := videoSpawnCalls.Load()
	if err := videoSpawn(nil); err != nil {
		t.Fatalf("videoSpawn() = %v, want nil", err)
	}
	if videoSpawnCalls.Load() != before+1 {
		t.Fatalf("videoSpawn did not count the call")
	}
}
