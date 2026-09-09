package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if err := runUpdateWith(&buf, []string{"alpha"}, r, newRunLogOutput()); err != nil {
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
	if err := runUpdateWith(&buf, []string{"alpha"}, r, newRunLogOutput()); err != nil {
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

// countCaptures is how many capture files in dir belong to subject.
func countCaptures(t *testing.T, dir, subject string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "__"+subject+".log") {
			n++
		}
	}
	return n
}

// TestCaptureKeepsFiftyRunsPerHost pins fleet's retention: a host's 51st run
// evicts its oldest capture, and no other host's history is touched. The
// shared default is 200, which is far more scrollback than an operator ever
// reads and made ~/.local/state/fleet/logs grow to hundreds of files; 50 runs
// per host is enough to answer "what changed since it last worked" while
// keeping the directory something you can actually list.
func TestCaptureKeepsFiftyRunsPerHost(t *testing.T) {
	dir := t.TempDir()

	// Names are timestamped to the second, so each capture needs its own.
	saved := nowFn
	defer func() { nowFn = saved }()
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { at = at.Add(time.Second); return at }

	out := captureOutput{dir: dir}
	for i := 0; i < 51; i++ {
		w, _ := out.Open("alpha", "run")
		w.Close("done")
	}
	w, _ := out.Open("beta", "run")
	w.Close("done")

	if got := countCaptures(t, dir, "alpha"); got != 50 {
		t.Errorf("alpha: kept %d captures, want 50", got)
	}
	if got := countCaptures(t, dir, "beta"); got != 1 {
		t.Errorf("beta: kept %d captures, want 1 — pruning must not cross hosts", got)
	}
}

// TestZeroValueCaptureOutputWritesNothing pins that a captureOutput with no
// dir — the zero value every test-built Executor and tuiModel produces —
// captures nothing rather than writing into the operator's real state
// directory. It used to resolve to <state>/fleet/logs, so `go test ./...`
// deposited three files in the developer's own ~/.local/state/fleet/logs on
// every run; 351 of the 384 files found there were named after test fixtures
// (h, host-a, alpha). Losing a capture is free (Open falls back to Discard);
// writing one somewhere nobody named is not.
func TestZeroValueCaptureOutputWritesNothing(t *testing.T) {
	// A state dir IS in scope — this is exactly where a stray file would go.
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	w, path := captureOutput{}.Open("alpha", "header")
	w.Line("some output")
	w.Close("done")

	if path != "" {
		t.Errorf("a dirless capture reported path %q; it must capture nothing", path)
	}
	if entries, err := os.ReadDir(filepath.Join(state, "fleet", "logs")); err == nil && len(entries) > 0 {
		t.Errorf("wrote %d file(s) into the real state dir: %v", len(entries), entries)
	}
}

// TestUpdateCapturesOnlyWhereTheCallerNamed pins that the CLI update path
// writes captures ONLY to a directory its caller chose. runUpdateWith injects
// its output writer and its runner but used to resolve the capture directory
// itself via fleetLogDir(), so every test driving the real CLI path deposited
// a file in the developer's own ~/.local/state/fleet/logs. This is the same
// rule tuiModel.ansPath already follows: the persistence path is INJECTED,
// never resolved inside the thing being tested.
func TestUpdateCapturesOnlyWhereTheCallerNamed(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	withUpdateFile(t, updplan.DefaultYAML)
	r := runner.Fake{Err: map[string]error{"alpha": fmt.Errorf("boom")}}
	var buf strings.Builder
	_ = runUpdateWith(&buf, []string{"alpha"}, r, updexec.Discard{})

	if entries, err := os.ReadDir(filepath.Join(state, "fleet", "logs")); err == nil && len(entries) > 0 {
		t.Fatalf("capture written to a directory the caller never named: %v", entries)
	}
}
