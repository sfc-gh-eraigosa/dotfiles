package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// This file holds the tests written against the final round of code-review
// findings on the fleet-update build's D+E leaves
// (docs/mbo/plans/fleet-update.md), one (or a pair) per finding, named for
// the defect they pin.

// --- finding 1: the assembled TUI preamble must be valid shell -------------

// TestRealTUIPreambleProducesValidShell builds the ACTUAL bgPreamble a
// background run step carries (sudo prime + sudoGate + exported answers)
// and asserts the assembled script parses under `sh -n` when joined onto a
// following command — the exact shape Console.runScript produces for a
// KindRun step. Before the fix, Console.runScript joined Preamble's text
// with " && " unconditionally, producing "... || exit 92;  && cd ..." — a
// shell syntax error on EVERY TUI background run step.
func TestRealTUIPreambleProducesValidShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -n requires a POSIX shell")
	}
	a := answers{windows: "s", gemini: "keep"}
	a.appendSecret("hunter2")
	preamble := bgPreamble(a)(updplan.Step{Kind: updplan.KindRun})

	script := preamble + "cd /tmp && true"
	if err := exec.Command("sh", "-n", "-c", script).Run(); err != nil {
		t.Fatalf("the assembled preamble+script is not valid shell: %v\nscript:\n%s", err, script)
	}
}

// --- finding 4: --json must still exit non-zero on a host failure ----------

// TestJSONOutputStillExitsNonZeroOnFailure pins the fix: runUpdateWith used
// to return printJSONReport's error (which only ever reports a
// marshal/write failure) under --json, so a failed host produced valid JSON
// on stdout but a 0 exit code — a script piping `fleet update --json` into
// `jq` and checking $? would never notice a failure.
func TestJSONOutputStillExitsNonZeroOnFailure(t *testing.T) {
	withUpdateFile(t, updplan.DefaultYAML)
	old := flagJSON
	flagJSON = true
	t.Cleanup(func() { flagJSON = old })

	r := runner.Fake{Err: map[string]error{"alpha": fmt.Errorf("boom")}}
	var buf strings.Builder
	err := runUpdateWith(&buf, []string{"alpha"}, r)
	if err == nil {
		t.Fatalf("a failed host under --json must still return a non-nil error, output:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "{") {
		t.Fatalf("--json must still print the JSON report even when the run failed:\n%s", buf.String())
	}
}

// --- finding 6: --update-ref must not override the plan's branch by default

// TestTUIDoesNotOverrideThePlanBranchByDefault pins the fix: resolveTUIPlan
// used to apply plan.WithRef(tuiUpdateRef) unconditionally with a default of
// "main", which silently overrode a [release] plan's own branch and refused
// to start at all against a multi-repo plan with no "dotfiles" repo (an
// ambiguous --ref target). An EMPTY ref (the flag's new default) must leave
// the plan untouched in both cases.
func TestTUIDoesNotOverrideThePlanBranchByDefault(t *testing.T) {
	dir := t.TempDir()

	t.Run("multi-repo plan with no dotfiles repo starts fine", func(t *testing.T) {
		yaml := `version: 1
update:
  repos:
    work: { path: work, branches: [release] }
    other: { path: other, branches: [main] }
  steps: []
`
		file := filepath.Join(dir, "multi.yaml")
		if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		p, err := resolveTUIPlan(file, "", dir)
		if err != nil {
			t.Fatalf("an empty --update-ref must never be applied via WithRef (which would refuse an ambiguous multi-repo target): %v", err)
		}
		if got := p.Repos["work"].Branches[0]; got != "release" {
			t.Fatalf("work.Branches[0] = %q, want release (untouched)", got)
		}
		if got := p.Repos["other"].Branches[0]; got != "main" {
			t.Fatalf("other.Branches[0] = %q, want main (untouched)", got)
		}
	})

	t.Run("dotfiles plan on a non-main branch stays there", func(t *testing.T) {
		yaml := `version: 1
update:
  repos:
    dotfiles: { path: dotfiles, branches: [release] }
  steps: []
`
		file := filepath.Join(dir, "release.yaml")
		if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		p, err := resolveTUIPlan(file, "", dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Repos["dotfiles"].Branches[0]; got != "release" {
			t.Fatalf("dotfiles.Branches[0] = %q, want release (must not be forced onto main)", got)
		}
	})
}

// TestTUIUpdateRefIsAppliedWhenGiven proves the operator can still target a
// different ref explicitly — only the DEFAULT (empty) must leave the plan
// alone.
func TestTUIUpdateRefIsAppliedWhenGiven(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "fleet.yaml")
	if err := os.WriteFile(file, []byte(updplan.DefaultYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := resolveTUIPlan(file, "hotfix", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Repos["dotfiles"].Branches[0]; got != "hotfix" {
		t.Fatalf("dotfiles.Branches[0] = %q, want hotfix", got)
	}
}

// --- finding 7: the headless capture must contain the remote's own output --

// TestHeadlessCaptureContainsRemoteOutput pins the fix: a headless `fleet
// update` run's capture used to contain only the executor's own step
// banners — never the remote command's own output (e.g. git's `fatal:`
// text) — because buildExecutor built a Console with no Line callback and
// nothing else teed Batch's output into the capture. The executor now tees
// every lane's output into the capture itself (Executor.RunHost).
func TestHeadlessCaptureContainsRemoteOutput(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	withUpdateFile(t, updplan.DefaultYAML)

	r := runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main\nfatal: unable to access remote"}}
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
	if !strings.Contains(string(data), "fatal: unable to access remote") {
		t.Fatalf("capture must contain the remote's own output, got:\n%s", data)
	}
}

// --- finding 8: the background Line feed must never block the executor -----

// TestBackgroundFeedNeverBlocksTheExecutor pushes far more lines than the
// old capacity-64 channel could hold with nobody reading, and asserts push
// still returns promptly — then proves a later reader drains everything, in
// order. Before the fix, Line pushed directly onto a bounded channel: once
// full, the executor's own goroutine (and the whole background update)
// stalled until the UI got back around to draining it — which it cannot do
// while tea.ExecProcess has suspended the event loop for another host's
// interactive handoff.
func TestBackgroundFeedNeverBlocksTheExecutor(t *testing.T) {
	q := newLineQueue()
	const n = 10000

	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			q.push(fmt.Sprintf("line-%d", i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("push blocked with no reader draining the queue")
	}

	ch := make(chan string)
	go q.forward(ch)
	q.closeQ()

	var got []string
	for l := range ch {
		got = append(got, l)
	}
	if len(got) != n {
		t.Fatalf("got %d lines, want %d", len(got), n)
	}
	for i, l := range got {
		if want := fmt.Sprintf("line-%d", i); l != want {
			t.Fatalf("out of order at index %d: got %q, want %q", i, l, want)
		}
	}
}

// --- finding 10: planLabel must keep the built-in marker and stay rune-safe

// TestPlanLabelKeepsTheBuiltInMarker pins the fix: planLabel used to
// truncate from the FRONT (keep only the tail), which for a long built-in
// Source like "built-in default (no /very/long/home/path/.config/fleet/
// fleet.yaml)" dropped the "built-in default (no " marker entirely — the
// status line then named a file that was NOT actually loaded.
func TestPlanLabelKeepsTheBuiltInMarker(t *testing.T) {
	long := "built-in default (no " + strings.Repeat("x", 60) + "/fleet.yaml)"
	got := planLabel(long)
	if !strings.HasPrefix(got, "built-in default") {
		t.Fatalf("planLabel(%q) = %q, must keep the built-in marker", long, got)
	}
}

// TestPlanLabelIsValidUTF8 proves a source whose truncation point would
// otherwise land mid-rune still produces valid UTF-8 output.
func TestPlanLabelIsValidUTF8(t *testing.T) {
	source := "/home/ops/héllo/wörld/" + strings.Repeat("日本語", 20) + "/fleet.yaml"
	got := planLabel(source)
	if !utf8.ValidString(got) {
		t.Fatalf("planLabel(%q) = %q, not valid UTF-8", source, got)
	}
}
