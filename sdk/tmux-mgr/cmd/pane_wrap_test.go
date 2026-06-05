package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunPaneWrap_ForwardsExitAndCheckpoints pins the orchestration: the
// agent's exit code is forwarded and the auto-checkpoint runs with the ref.
func TestRunPaneWrap_ForwardsExitAndCheckpoints(t *testing.T) {
	for _, want := range []int{0, 3} {
		var gotRef string
		var stderr bytes.Buffer
		code := runPaneWrap("auth/erai/api", []string{"agent", "--flag"}, paneWrapDeps{
			runChild:   func(argv []string) int { return want },
			checkpoint: func(ref string) error { gotRef = ref; return nil },
			stderr:     &stderr,
		})
		if code != want {
			t.Errorf("exit code = %d; want %d (forwarded from agent)", code, want)
		}
		if gotRef != "auth/erai/api" {
			t.Errorf("checkpoint ref = %q; want auth/erai/api", gotRef)
		}
		if stderr.Len() != 0 {
			t.Errorf("unexpected stderr: %s", stderr.String())
		}
	}
}

// TestRunPaneWrap_CheckpointFailureDoesNotOverrideExit: a checkpoint error is
// logged but the agent's exit code still wins.
func TestRunPaneWrap_CheckpointFailureDoesNotOverrideExit(t *testing.T) {
	var stderr bytes.Buffer
	code := runPaneWrap("auth/erai/api", []string{"agent"}, paneWrapDeps{
		runChild:   func(argv []string) int { return 0 },
		checkpoint: func(ref string) error { return errors.New("skip: dirty") },
		stderr:     &stderr,
	})
	if code != 0 {
		t.Errorf("exit code = %d; want 0 (checkpoint failure must not override)", code)
	}
	if !strings.Contains(stderr.String(), "auto-checkpoint failed") {
		t.Errorf("want a checkpoint-failure notice on stderr; got %q", stderr.String())
	}
}

// TestExecChild_RealExitCodes exercises the real exec path + exit-code mapping.
func TestExecChild_RealExitCodes(t *testing.T) {
	if got := execChild([]string{"/bin/sh", "-c", "exit 0"}); got != 0 {
		t.Errorf("exit 0: got %d", got)
	}
	if got := execChild([]string{"/bin/sh", "-c", "exit 7"}); got != 7 {
		t.Errorf("exit 7: got %d", got)
	}
	if got := execChild([]string{"/no/such/binary/xyzzy"}); got != 127 {
		t.Errorf("missing binary: got %d; want 127", got)
	}
}

// TestPaneWrap_EndToEnd is a near-e2e: a real agent (/bin/sh) plus a fake gss
// on PATH that records its invocation (the acceptance smoke, sans tmux).
func TestPaneWrap_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "gss-args.txt")
	fakeGss := filepath.Join(dir, "gss")
	script := "#!/bin/sh\necho \"$@\" > \"" + record + "\"\n"
	if err := os.WriteFile(fakeGss, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	code := runPaneWrap("auth/erai/api", []string{"/bin/sh", "-c", "exit 0"}, paneWrapDeps{
		runChild:   execChild,
		checkpoint: runGssCheckpoint,
		stderr:     os.Stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("fake gss was not invoked: %v", err)
	}
	if want := "feature checkpoint --auto --worker auth/erai/api"; strings.TrimSpace(string(got)) != want {
		t.Errorf("gss invoked with %q; want %q", strings.TrimSpace(string(got)), want)
	}
}

func TestPaneWrapCmd_Wired(t *testing.T) {
	if !hasSub(internalCmd, "pane-wrap") {
		t.Error("internal pane-wrap not wired")
	}
	if !hasSub(rootCmd, "internal") {
		t.Error("internal not registered on root")
	}
}

func hasSub(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}
