package cmd

import (
	"strings"
	"testing"
)

// P0 / sdk checklist #2: build.sh stamps the version via -ldflags. The binary
// must report it rather than a placeholder.
func TestVersionString_ReportsStampedValues(t *testing.T) {
	Version, Commit = "1.2.3", "abc1234"
	got := VersionString()
	for _, want := range []string{"1.2.3", "abc1234"} {
		if !strings.Contains(got, want) {
			t.Errorf("VersionString() = %q, want it to contain %q", got, want)
		}
	}
}

// P0 / sdk/AGENTS.md logging contract: every tool logs through sdk/libs/log,
// and must claim its tool name exactly once at startup so log records are
// attributable. Nothing may log before that happens.
func TestInitLogging_SetsToolNameOnce(t *testing.T) {
	initLogging()
	if loggingTool != "wlink" {
		t.Errorf("loggingTool = %q, want %q", loggingTool, "wlink")
	}
	if initLoggingCalls != 1 {
		t.Errorf("initLogging ran %d times, want exactly 1", initLoggingCalls)
	}
	initLogging()
	if initLoggingCalls != 1 {
		t.Errorf("initLogging is not idempotent: ran %d times", initLoggingCalls)
	}
}
