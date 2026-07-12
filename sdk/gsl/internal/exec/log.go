package exec

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sirupsen/logrus"
)

// LogSubprocessError emits a seam's structured failure record at the level the
// failure DESERVES. seam is "git" | "gh" | "mcp"; the event key is
// "<seam>.subprocess_error", unchanged, so existing log consumers keep working.
//
// Why the level matters: a routine non-zero exit ("not a git repository", "no
// pull requests found for branch") is the NORMAL case on a hot path that runs
// on every assistant turn. Logged at Warn it produced ~1.6k records per session
// and buried the handful of segment.panic entries that mean gsl is actually
// broken. Routine exits are now Debug; genuine operational failures — a
// timeout, a WaitDelay overrun from a leaked orphan, a missing binary — stay at
// Warn. See IsExpectedExit for the exact split.
func LogSubprocessError(seam, bin string, args []string, err error) {
	entry := observe.Default().WithFields(logrus.Fields{
		"event":   seam + ".subprocess_error",
		"command": bin,
		"args":    args,
		"error":   err.Error(),
	})
	if IsExpectedExit(err) {
		entry.Debug(seam + " subprocess exited non-zero (expected; degrading)")
		return
	}
	entry.Warn(seam + " subprocess failed")
}
