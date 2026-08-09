package exec_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gslexec "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/exec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
)

// TestIsExpectedExit_RoutineNonZeroExit — `git status` outside a repo, `gh pr
// view` with no PR: the command ran and answered "no". Routine ⇒ Debug.
func TestIsExpectedExit_RoutineNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	err := exec.Command("/bin/sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("expected a non-zero exit")
	}
	if !gslexec.IsExpectedExit(err) {
		t.Errorf("IsExpectedExit(exit 1) = false; want true (a routine non-zero exit is "+
			"the NORMAL case on the hot path and must not be logged at Warn). err=%v", err)
	}
}

// TestIsExpectedExit_Success / _Nil — no error is not an "expected exit".
func TestIsExpectedExit_Nil(t *testing.T) {
	if gslexec.IsExpectedExit(nil) {
		t.Error("IsExpectedExit(nil) = true; want false")
	}
	if gslexec.IsExpectedExit(errors.New("some other error")) {
		t.Error("IsExpectedExit(plain error) = true; want false")
	}
}

// TestIsExpectedExit_SignalDeath is the load-bearing exclusion: a process WE
// killed on cancellation must NOT be filed as a routine exit — that is the
// timeout we are trying to make visible.
func TestIsExpectedExit_SignalDeath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5")
	gslexec.Harden(cmd)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected an error from the cancelled command")
	}
	if gslexec.IsExpectedExit(err) {
		t.Errorf("IsExpectedExit(signal death) = true; want false — a process killed by "+
			"our own deadline is an operational failure (Warn), not a routine 'no' (Debug). err=%v", err)
	}
}

// TestIsExpectedExit_MissingBinary — a missing gh/claude is a real
// misconfiguration and stays at Warn.
func TestIsExpectedExit_MissingBinary(t *testing.T) {
	err := exec.Command("gsl-definitely-not-a-real-binary-xyz").Run()
	if err == nil {
		t.Fatal("expected a lookup error")
	}
	if gslexec.IsExpectedExit(err) {
		t.Errorf("IsExpectedExit(missing binary) = true; want false. err=%v", err)
	}
}

// TestLogSubprocessError_Levels pins the split end to end through the logger.
func TestLogSubprocessError_Levels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}

	read := func(t *testing.T, f func()) string {
		t.Helper()
		logPath := filepath.Join(t.TempDir(), "gsl.log")
		t.Setenv("GSL_LOG_FILE", logPath)
		t.Setenv("GSL_LOG_LEVEL", "debug")
		observe.ResetDefaultForTest()
		t.Cleanup(observe.ResetDefaultForTest)
		f()
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		return string(data)
	}

	t.Run("routine exit is debug", func(t *testing.T) {
		err := exec.Command("/bin/sh", "-c", "exit 1").Run()
		got := read(t, func() {
			gslexec.LogSubprocessError("git", "git", []string{"status"}, err)
		})
		if !strings.Contains(got, `"event":"git.subprocess_error"`) {
			t.Fatalf("missing event record: %s", got)
		}
		if !strings.Contains(got, `"level":"debug"`) {
			t.Errorf("want level=debug for a routine non-zero exit; got: %s", got)
		}
	})

	t.Run("real failure is warn", func(t *testing.T) {
		got := read(t, func() {
			gslexec.LogSubprocessError("gh", "gh", []string{"pr", "view"}, context.DeadlineExceeded)
		})
		if !strings.Contains(got, `"level":"warning"`) {
			t.Errorf("want level=warning for a deadline; got: %s", got)
		}
	})
}
