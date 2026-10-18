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
func TestMain(m *testing.M) {
	videoSpawn = func() error {
		videoSpawnCalls.Add(1)
		return nil
	}
	os.Exit(m.Run())
}

func TestVideoSpawn_IsNoOpUnderTest(t *testing.T) {
	before := videoSpawnCalls.Load()
	if err := videoSpawn(); err != nil {
		t.Fatalf("videoSpawn() = %v, want nil", err)
	}
	if videoSpawnCalls.Load() != before+1 {
		t.Fatalf("videoSpawn did not count the call")
	}
}
