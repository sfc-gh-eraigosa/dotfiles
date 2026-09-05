package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// withUpdateFile points --file at a temp plan file for the duration of one
// test, so plan resolution never touches gff or a real $HOME/$XDG dir.
func withUpdateFile(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fleet.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	old := flagUpdateFile
	flagUpdateFile = path
	t.Cleanup(func() { flagUpdateFile = old })
	return path
}

// TestHeadlessUpdateIsCaptured mirrors runlog_test.go's approach for the CLI
// path: XDG_STATE_HOME points the capture at a temp dir, and the resulting
// file must carry the executor's header (host=/plan=/mode=).
func TestHeadlessUpdateIsCaptured(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	withUpdateFile(t, updplan.DefaultYAML)

	r := runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}
	var buf strings.Builder
	if err := runUpdateWith(&buf, []string{"alpha"}, r); err != nil {
		t.Fatalf("unexpected error: %v\noutput:\n%s", err, buf.String())
	}

	logDir := filepath.Join(stateDir, "fleet", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected a capture under %s, got err=%v entries=%v", logDir, err, entries)
	}
	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"host=alpha", "plan=", "mode=fast-forward"} {
		if !strings.Contains(s, want) {
			t.Fatalf("capture missing %q:\n%s", want, s)
		}
	}
}

// A capture that cannot be opened must never cost the update: an unusable
// state dir still leaves the run succeeding, just uncaptured.
func TestAnUnusableCaptureDirDoesNotBreakTheCLIRun(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/proc/cannot/mkdir/here")
	withUpdateFile(t, updplan.DefaultYAML)

	r := runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}
	var buf strings.Builder
	if err := runUpdateWith(&buf, []string{"alpha"}, r); err != nil {
		t.Fatalf("a lost capture must not fail the update: %v", err)
	}
}

// The local environment's pre-supplied install.sh answers must reach a run
// step's script, and ONLY a run step's — never sync or gh-auth.
func TestLocalAnswerEnvIsExportedForRunStepsOnly(t *testing.T) {
	t.Setenv("WINSETUP_ANSWER", "s")
	t.Setenv("GEMINI_TEARDOWN_ANSWER", "keep")

	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}, log: &sent}

	plan := updplan.Plan{
		Steps: []updplan.Step{
			{ID: "sync1", Kind: updplan.KindSync, Repo: "r", Expect: updplan.Expect{Exit: []int{0}}},
			{ID: "run1", Kind: updplan.KindRun, Run: "echo hi", Needs: []string{"sync1"}, Expect: updplan.Expect{Exit: []int{0}}},
		},
		Repos: map[string]updplan.Repo{
			"r": {Name: "r", Path: "~/git/r", Branches: []string{"main"}, Local: updplan.LocalSkip, Restore: true},
		},
	}

	ex := buildExecutor(r, nil, "")
	rep := ex.RunHost("alpha", plan)
	if rep.Failed() {
		t.Fatalf("unexpected failure: %+v", rep.Results)
	}

	var withEnv int
	for _, s := range sent {
		if strings.Contains(s, "WINSETUP_ANSWER=s") && strings.Contains(s, "GEMINI_TEARDOWN_ANSWER=keep") {
			withEnv++
		}
	}
	if withEnv != 1 {
		t.Fatalf("expected exactly 1 script (the run step) carrying the answer env, got %d:\n%v", withEnv, sent)
	}
}

// The CLI lane never supplies Stdin at all — it has no sudo secret to send,
// unlike the TUI's Background lane. Extends
// TestSudoSecretNeverAppearsInTheRemoteCommand (tui_answers_test.go), which
// covers the TUI's env-only contract; this covers the CLI's have-no-stdin
// contract specifically.
func TestCLILaneCarriesNoStdinAtAll(t *testing.T) {
	ex := buildExecutor(runner.Fake{}, nil, "")
	c, ok := ex.IO.(updexec.Console)
	if !ok {
		t.Fatalf("IO is %T, want updexec.Console", ex.IO)
	}
	if c.Stdin != nil {
		t.Fatal("the CLI lane must never supply Stdin — it has no credential to send")
	}
}

// TestNewRunLogOutputIsReusableAcrossHosts pins that a single newRunLogOutput()
// value carries no per-host state and can be shared across every host's
// RunHost call in one run: two Open calls against the SAME Output value,
// for two different hosts, must produce two independent captures rather
// than colliding or requiring a fresh Output per host.
func TestNewRunLogOutputIsReusableAcrossHosts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	out := newRunLogOutput()
	w1, path1 := out.Open("alpha", "header for alpha")
	w1.Close("finished")
	w2, path2 := out.Open("beta", "header for beta")
	w2.Close("finished")

	if path1 == "" || path2 == "" || path1 == path2 {
		t.Fatalf("expected two distinct capture paths, got %q and %q", path1, path2)
	}
}
