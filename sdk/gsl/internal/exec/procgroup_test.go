package exec_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gslexec "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/exec"
)

// TestHarden_SetsWaitDelayAndCancel asserts the three knobs are actually set.
// They only work together: Setpgid makes the group addressable, Cancel kills it,
// WaitDelay is the backstop for an orphan we could not reach.
func TestHarden_SetsWaitDelayAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "true")
	gslexec.Harden(cmd)

	if cmd.WaitDelay != gslexec.DefaultWaitDelay {
		t.Errorf("WaitDelay = %v; want %v — without it, Wait() blocks on a pipe an "+
			"orphaned grandchild still holds (E18)", cmd.WaitDelay, gslexec.DefaultWaitDelay)
	}
	if cmd.Cancel == nil {
		t.Error("Cancel is nil; ctx cancellation would kill only the direct child")
	}
	if runtime.GOOS != "windows" {
		if cmd.SysProcAttr == nil {
			t.Fatal("SysProcAttr is nil; Setpgid was not applied and the grandchildren " +
				"are in gsl's OWN process group — a negative-pid kill would signal gsl itself")
		}
	}
}

// TestHarden_NilCmd_NoPanic — a nil cmd is a no-op, not a crash.
func TestHarden_NilCmd_NoPanic(t *testing.T) {
	gslexec.Harden(nil)
	gslexec.HardenWithDelay(nil, time.Second)
}

// TestHardenWithDelay_ZeroLeavesDefault documents that a non-positive delay does
// NOT set WaitDelay (0 means "block until EOF", the very behaviour E18 forbids),
// so a caller must pass a positive value to get the guarantee.
func TestHardenWithDelay_ZeroLeavesDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "true")
	gslexec.HardenWithDelay(cmd, 0)
	if cmd.WaitDelay != 0 {
		t.Errorf("WaitDelay = %v; want 0 (unchanged)", cmd.WaitDelay)
	}

	cmd2 := exec.CommandContext(ctx, "/bin/sh", "-c", "true")
	gslexec.HardenWithDelay(cmd2, 50*time.Millisecond)
	if cmd2.WaitDelay != 50*time.Millisecond {
		t.Errorf("WaitDelay = %v; want 50ms", cmd2.WaitDelay)
	}
}

// TestKillGroup_NotStarted returns ErrProcessDone rather than panicking on a
// command with no Process.
func TestKillGroup_NotStarted(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "true")
	if err := gslexec.KillGroup(cmd); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("KillGroup(unstarted) = %v; want os.ErrProcessDone", err)
	}
	if err := gslexec.KillGroup(nil); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("KillGroup(nil) = %v; want os.ErrProcessDone", err)
	}
}

// TestKillGroup_KillsGrandchild is the mechanism test behind E18: the whole
// process GROUP dies, not just the direct child.
//
// The grandchild sleeps 1s and then touches a marker. We kill the group ~100ms
// in. If only the direct child (sh) were killed, the orphaned grandchild would
// survive and create the marker.
func TestKillGroup_KillsGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process groups; Windows uses Job Objects (out of scope)")
	}

	marker := filepath.Join(t.TempDir(), "grandchild-survived")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		"( sleep 1; touch "+marker+" ) & sleep 30")
	gslexec.Harden(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if err := gslexec.KillGroup(cmd); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("KillGroup: %v", err)
	}
	_ = cmd.Wait()

	// Well past the grandchild's 1s sleep: prove it is dead, not merely slow.
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("the grandchild outlived the group kill and wrote %s — only the "+
			"direct child was killed (E18/F12)", marker)
	}
}
