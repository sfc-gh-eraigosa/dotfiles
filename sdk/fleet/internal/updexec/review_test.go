package updexec

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

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// This file holds the tests written against the leaf B code-review findings
// (docs/mbo/plans/fleet-update.md), one (or a pair) per finding, named for
// the defect they pin.

// --- finding 1: --reset must not land on FETCH_HEAD in the multi/default forms

// TestResetInMultiAndDefaultFormsTargetsOriginB1NotFetchHead reproduces the
// review finding: after a bare `git fetch origin` (default form) or a
// multi-ref `git fetch origin B1 B2` (multi form), FETCH_HEAD is whichever
// ref was last advertised — typically the clone's CURRENT branch's
// upstream, not necessarily origin/$b1. `--reset` must land on
// `origin/$b1` explicitly in both forms; only the single-branch form (whose
// `git fetch origin B1` makes FETCH_HEAD exactly origin/B1) may keep
// FETCH_HEAD.
func TestResetInMultiAndDefaultFormsTargetsOriginB1NotFetchHead(t *testing.T) {
	cases := []struct {
		name     string
		branches []string
	}{
		{"multi", []string{"main", "staging"}},
		{"default", []string{"default"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mustSync(t, repo("dotfiles", c.branches), updplan.LocalSkip, true)
			if !strings.Contains(s, `git reset --hard "origin/$b1"`) {
				t.Fatalf("%s form under --reset must reset to \"origin/$b1\":\n%s", c.name, s)
			}
			if strings.Contains(s, "git reset --hard FETCH_HEAD") {
				t.Fatalf("%s form under --reset must NOT reset to FETCH_HEAD (it is not necessarily origin/$b1 after this fetch):\n%s", c.name, s)
			}
		})
	}
}

// TestResetInSingleBranchFormKeepsFetchHead pins that the single-branch
// form's --reset text is frozen: `git fetch origin B1` makes FETCH_HEAD
// exactly origin/B1, so resetting to it is correct there (unlike the
// multi/default forms above).
func TestResetInSingleBranchFormKeepsFetchHead(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main"}), updplan.LocalSkip, true)
	if !strings.Contains(s, "git reset --hard FETCH_HEAD") {
		t.Fatalf("single-branch form under --reset must keep FETCH_HEAD:\n%s", s)
	}
}

// --- finding 2: the default-branch sync body must not swallow a failed fetch

// TestDefaultSyncFetchStaysInTheAndChain proves the fetch is followed by
// `&&`, never `;`: a `;` there let a failed fetch fall through to
// resolving/checking out/merging stale refs and exit 0.
func TestDefaultSyncFetchStaysInTheAndChain(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"default"}), updplan.LocalSkip, false)
	idx := strings.Index(s, "git fetch origin")
	if idx < 0 {
		t.Fatalf("no fetch found:\n%s", s)
	}
	after := s[idx+len("git fetch origin"):]
	if !strings.HasPrefix(after, " && ") {
		t.Fatalf("the fetch must be followed by && (not ;):\n%s", s)
	}
}

// stubBin installs a fake executable named name on PATH, so a test can
// prove behaviour under a controlled `git` without opening a socket or a
// real repository. Mirrors runner.stubSSH's technique.
func stubBin(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub script requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestDefaultSyncFailsWhenFetchFails reproduces the review finding directly:
// under a stub `git` whose `fetch` exits 1 but whose `ls-remote --symref`
// FALLBACK (the local symbolic-ref probe also stubbed to fail) still
// resolves a plausible default branch, the whole default-form sync script
// must exit non-zero — never fall through to checking out and merging that
// stale/wrong resolution and exiting 0. The ls-remote fallback is
// deliberately made to SUCCEED here: a stub that fails it too would exit
// non-zero via the script's own pre-existing "cannot resolve the default
// branch" guard regardless of this fix, and prove nothing about the actual
// bug (a failed fetch not gating the branch-resolution/checkout/merge that
// follows it).
func TestDefaultSyncFailsWhenFetchFails(t *testing.T) {
	stubBin(t, "git", `case "$1" in
  fetch) exit 1 ;;
  symbolic-ref) exit 1 ;;
  rev-parse) echo deadbeefdeadbeefdeadbeefdeadbeefdeadbeef; exit 0 ;;
  ls-remote) printf 'ref: refs/heads/main\tHEAD\n'; exit 0 ;;
  *) exit 0 ;;
esac`)

	work := t.TempDir()
	r := repo("dotfiles", []string{"default"})
	r.Path = work
	s := mustSync(t, r, updplan.LocalSkip, false)

	if err := exec.Command("sh", "-c", s).Run(); err == nil {
		t.Fatal("a failed fetch must fail the whole default-form sync script, even though the ls-remote fallback successfully resolved a branch — it must never checkout/merge after a failed fetch")
	}
}

// --- finding 3: carry notes must survive a retry

// TestCarryNotesSurviveARetry pins the fix for the finding: the carry
// prologue (stash push, branch switch) runs on EVERY attempt, so a stash
// pushed and a branch switched by a failed first attempt must still be
// visible to the restore machinery once a later attempt succeeds. Before
// the fix, only the LAST attempt's Notes survived, so the second attempt's
// bare "fleet: orig=main" (no carry/switch lines, because the clone was
// already stashed and switched by the first attempt) meant the restore was
// never armed at all — a stranded stash with no restore.
func TestCarryNotesSurviveARetry(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=feature", nil). // attempt 1's precheck
		on("git rev-parse --git-dir", "state=clean branch=main", nil).    // attempt 2's precheck: already switched by attempt 1
		on("orig=$(git symbolic-ref",
			"fleet: orig=feature\nfleet: carried stash="+sha+" from=feature\nfleet: switched feature -> main",
									ErrTransport). // attempt 1: carried + switched, then a transport-classified failure
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil). // attempt 2: already on main, succeeds
		on(restoreFailedMarker, "", nil)

	p := onlySyncPlan(updplan.LocalCarry, false)
	st := p.Steps[0]
	st.Retry = updplan.Retry{Attempts: 2, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: updplan.Backoff{Initial: time.Millisecond, Factor: 1}}
	p.Steps[0] = st

	e := Executor{IO: io, Sleep: func(time.Duration) {}}
	rep := e.RunHost("alpha", p)

	sync, ok := resultFor(rep, "dotfiles.sync")
	if !ok || sync.Status != OK || sync.Attempts != 2 {
		t.Fatalf("sync = %+v, want ok after 2 attempts", sync)
	}

	rr, ok := resultFor(rep, "dotfiles.restore")
	if !ok || rr.Status != OK {
		t.Fatalf("a stash carried on the FIRST attempt must still arm a restore after a later attempt succeeds: %+v", rr)
	}

	var restoreScript string
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, restoreFailedMarker) {
			restoreScript = c.script
		}
	}
	if !strings.Contains(restoreScript, "git checkout -q feature") {
		t.Fatalf("restore must check out the ORIGINAL branch recorded on the first attempt: %q", restoreScript)
	}
	if !strings.Contains(restoreScript, "stash apply -q "+sha) {
		t.Fatalf("restore must apply the stash carried on the first attempt: %q", restoreScript)
	}
}

// --- finding 4: RescueScript's orig on a detached HEAD, and a tolerant commit

// TestRescueRecordsOrigViaSymbolicRefNotAbbrevRef pins the fix: on a
// detached checkout, `git rev-parse --abbrev-ref HEAD` prints the literal
// string "HEAD", making the later `git checkout -q "$orig"` a no-op and
// stranding the clone on fleet-rescue/<ts> once `git worktree add` for that
// SAME branch fails.
func TestRescueRecordsOrigViaSymbolicRefNotAbbrevRef(t *testing.T) {
	s, err := RescueScript(repo("dotfiles", []string{"main"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD)`) {
		t.Fatalf("orig must be recorded via symbolic-ref, falling back to the SHA:\n%s", s)
	}
	if strings.Contains(s, "--abbrev-ref") {
		t.Fatalf("orig must never use --abbrev-ref HEAD (prints the literal string \"HEAD\" when detached):\n%s", s)
	}
}

// TestRescueCommitToleratesNothingToCommit pins the || true guard, mirroring
// ResetScript: submodule-only dirt stages nothing under `git add -A`, and
// without `|| true` that aborts the && chain, leaving the clone stranded on
// fleet-rescue/<ts>.
func TestRescueCommitToleratesNothingToCommit(t *testing.T) {
	s, err := RescueScript(repo("dotfiles", []string{"main"}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{ git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet rescue $ts" || true; }`
	if !strings.Contains(s, want) {
		t.Fatalf("rescue commit must tolerate nothing-to-commit like ResetScript's does:\n%s", s)
	}
}

// --- finding 6: NoRestore / repo.Restore must be honoured on the immediate path too

// TestNoRestoreIsHonouredWhenTheSyncFails pins the fix: armAndMaybeRestoreNow
// (the IMMEDIATE restore path, taken when a sync fails after it already
// stashed/switched) used to run the restore unconditionally, never
// consulting Executor.NoRestore or repo.Restore — only the DEFERRED path
// (fireDueRestores) checked them.
func TestNoRestoreIsHonouredWhenTheSyncFails(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash="+sha+" from=main", os.ErrDeadlineExceeded).
		on(restoreFailedMarker, "", nil)
	e := Executor{IO: io, NoRestore: true}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalCarry, false))

	sync, ok := resultFor(rep, "dotfiles.sync")
	if !ok || sync.Status != Failed {
		t.Fatalf("sync = %+v, want failed", sync)
	}
	if _, ok := resultFor(rep, "dotfiles.restore"); ok {
		t.Fatal("--no-restore must be honoured even on the immediate (sync-failed) restore path")
	}
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, restoreFailedMarker) {
			t.Fatalf("--no-restore must never send a restore script, got: %q", c.script)
		}
	}
	found := false
	for _, n := range sync.Notes {
		if strings.Contains(n, "restore disabled") && strings.Contains(n, sha) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the sync step's own Notes must record the kept stash: %+v", sync.Notes)
	}
}

// --- finding 7: a failing non-last EXTRAS iteration must fail the step

// TestExtrasLoopTracksFailureAcrossIterations is a pure-string assertion
// that the loop tracks a fail flag rather than trusting its own (POSIX
// `for`, last-iteration-only) exit status.
//
// Updated for the D+E finding: the whole thing is now wrapped in `{ …; }`
// so a caller can join it onto a preceding fetch/checkout/merge with a
// single `&&` and have the ENTIRE loop gated by that chain — previously
// only the leading `fail=0` assignment was gated (it followed the `&&`) and
// the `for` loop itself followed a bare `;`, so it ran even when the move
// before it had failed, and `[ "$fail" = 0 ]` then replaced that failure's
// real exit code with a generic 1. This test used to pin the un-braced
// (buggy) text; it is now fixed to assert the braced form.
func TestExtrasLoopTracksFailureAcrossIterations(t *testing.T) {
	s := extrasScript([]string{"staging", "release"})
	if !strings.HasPrefix(s, "{ fail=0; for b in staging release; do") {
		t.Fatalf("extras must be wrapped in { … } with a fail flag initialised before the loop:\n%s", s)
	}
	if !strings.Contains(s, `echo "fleet: ff $b" || fail=1`) {
		t.Fatalf("a failing force-move must set the fail flag:\n%s", s)
	}
	if !strings.Contains(s, `echo "fleet: created $b" || fail=1`) {
		t.Fatalf("a failing track-create must set the fail flag:\n%s", s)
	}
	if !strings.HasSuffix(s, `done; [ "$fail" = 0 ]; }`) {
		t.Fatalf("the loop must end by checking the fail flag and closing the brace group:\n%s", s)
	}
}

// TestExtrasScriptJoinsOntoTheAndChainAsOneUnit pins the fix directly:
// syncBody/CloneScript join extrasScript onto the preceding move with
// " && ", and extrasScript's own braces are what make that join gate the
// WHOLE loop rather than just its first statement.
func TestExtrasScriptJoinsOntoTheAndChainAsOneUnit(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main", "staging"}), updplan.LocalSkip, false)
	if !strings.Contains(s, `&& { fail=0;`) {
		t.Fatalf("extras must be joined onto the preceding move with && { fail=0; …:\n%s", s)
	}
}

// TestExtrasLoopNeverRunsAfterAFailedMergeUpstream reproduces the finding
// with a stub git whose merge fails (128): the extras loop's own `git
// branch` must NEVER run, and the script's exit code must remain 128 (the
// merge's own code), not the loop's masking 1.
func TestExtrasLoopNeverRunsAfterAFailedMergeUpstream(t *testing.T) {
	work := t.TempDir()
	logFile := filepath.Join(work, "branch.log")
	stubBin(t, "git", `
case "$1" in
  fetch) exit 0 ;;
  checkout) exit 0 ;;
  merge) exit 128 ;;
  symbolic-ref) echo main; exit 0 ;;
  rev-parse) echo deadbeefdeadbeefdeadbeefdeadbeefdeadbeef; exit 0 ;;
  branch) echo "$@" >> `+ShQuote(logFile)+`; exit 0 ;;
esac
exit 0`)

	r := repo("dotfiles", []string{"main", "staging"})
	r.Path = work
	s := mustSync(t, r, updplan.LocalSkip, false)

	err := exec.Command("sh", "-c", s).Run()
	if err == nil {
		t.Fatal("a failed merge must fail the whole sync script")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 128 {
		t.Fatalf("exit code = %v, want 128 (the merge's own code, not the loop's masking 1)", err)
	}
	if _, statErr := os.Stat(logFile); statErr == nil {
		t.Fatal("the extras loop's git branch must never run after a failed merge")
	}
}

// TestExtrasFailureFailsTheStepEvenWhenALaterExtraSucceeds reproduces the
// finding with a stub `git`: the FIRST extra fails (a missing remote
// branch), the SECOND succeeds — under POSIX `for`'s last-iteration-only
// exit status this used to report the step ok.
func TestExtrasFailureFailsTheStepEvenWhenALaterExtraSucceeds(t *testing.T) {
	stubBin(t, "git", `case "$1 $2" in
  "show-ref -q") exit 1 ;;
esac
case "$*" in
  *"track b1x"*) exit 1 ;;
esac
exit 0`)

	s := extrasScript([]string{"b1x", "b2x"})
	err := exec.Command("sh", "-c", "b1=main; "+s).Run()
	if err == nil {
		t.Fatal("a failing non-last extra must fail the step, even though a later extra succeeds")
	}
}

// --- finding 8: Console.Interactive must not race a ctx deadline against RunInteractive

// TestInteractiveDeadlineKillsTheChild proves that when the runner
// implements interactiveCtxRunner (runner.Exec does), a deadline actually
// kills the remote child rather than merely abandoning it in a goroutine.
func TestInteractiveDeadlineKillsTheChild(t *testing.T) {
	stubBin(t, "ssh", "exec sleep 30") // exec: the killed PID is the sleeper itself, so no orphan inherits our stdout and holds the test binary open for 30s

	c := Console{R: runner.Exec{}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Interactive(ctx, "host", updplan.Step{ID: "x", Kind: updplan.KindRun}, "echo hi")
	}()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error from a killed interactive child")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Console.Interactive did not honour the deadline within 2s — the child was never killed")
	}
}

// TestInteractiveWithoutCtxRunnerRunsUnboundedAndNotes proves that when the
// runner does NOT implement interactiveCtxRunner (recordingRunner does
// not), Console.Interactive runs unbounded rather than racing a goroutine
// against the deadline — an already-expired deadline must not turn a
// successful RunInteractive into a spurious context.DeadlineExceeded.
func TestInteractiveWithoutCtxRunnerRunsUnboundedAndNotes(t *testing.T) {
	r := &recordingRunner{}
	c := Console{R: r}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // ensure the deadline has already lapsed

	err := c.Interactive(ctx, "alpha", updplan.Step{ID: "x", Kind: updplan.KindRun}, "script")
	if err != nil {
		t.Fatalf("a runner lacking RunInteractiveCtx must run unbounded, ignoring the expired deadline: %v", err)
	}
	if r.count("RunInteractive") != 1 {
		t.Fatalf("must fall back to RunInteractive exactly once, got %d", r.count("RunInteractive"))
	}
}

// --- finding 9: Executor.Rand's doc-promised default

// TestJitterDefaultsToRandomnessWhenRandIsNil pins the fix: Executor.Rand's
// comment promises "nil -> math/rand", but rand() used to return e.Rand
// verbatim, so a nil Rand fed Backoff.Wait a nil rnd, which Backoff.Wait
// itself treats as the constant midpoint 0.5 — no jitter movement AT ALL in
// production.
func TestJitterDefaultsToRandomnessWhenRandIsNil(t *testing.T) {
	e := Executor{}
	f := e.rand()
	if f == nil {
		t.Fatal("rand() must never return nil")
	}
	seen := map[float64]bool{}
	for i := 0; i < 20; i++ {
		v := f()
		if v < 0 || v >= 1 {
			t.Fatalf("rand() returned %v, want a value in [0,1)", v)
		}
		seen[v] = true
	}
	if len(seen) < 2 {
		t.Fatalf("rand() must vary across calls when Rand is nil, got a constant value: %v", seen)
	}
}

// --- finding 10: timedOut must be derived from the call's own error, not a stale ctx.Err()

// expiresCtxIO simulates a step whose remote call finishes successfully
// (ssh exit 0) but whose deadline lapses in the window between the call
// returning and runWithRetry checking ctx.Err() — the output-drain window
// the finding describes.
type expiresCtxIO struct{}

func (expiresCtxIO) Batch(ctx context.Context, host string, st updplan.Step, script string) (string, error) {
	<-time.After(50 * time.Millisecond)
	return "ok", nil
}
func (expiresCtxIO) Interactive(ctx context.Context, host string, st updplan.Step, script string) error {
	return nil
}

// TestASuccessfulAttemptIsNeverReportedAsTimedOut pins the fix: a call that
// returns nil error is `ok` regardless of ctx.Err() by the time it returns.
func TestASuccessfulAttemptIsNeverReportedAsTimedOut(t *testing.T) {
	st := runStep("x", "", "echo hi")
	st.Timeout = 10 * time.Millisecond
	e := Executor{IO: expiresCtxIO{}}
	rep := e.RunHost("alpha", updplan.Plan{Steps: []updplan.Step{st}})

	r, ok := resultFor(rep, "x")
	if !ok {
		t.Fatal("missing result for step x")
	}
	if r.TimedOut {
		t.Fatalf("a successful attempt must never be reported TimedOut, even if its deadline lapsed while draining output: %+v", r)
	}
	if r.Status != OK {
		t.Fatalf("r.Status = %v, want OK: %+v", r.Status, r)
	}
}

// --- dropped-at-cap item: exec.ErrWaitDelay on a successful child is not a failure

// TestBatchTreatsBareErrWaitDelayAsSuccess pins the fix: exec.ErrWaitDelay
// on its OWN (not joined with a real *exec.ExitError) means the remote
// command's own exit was 0 and only the output-draining goroutine was still
// running when WaitDelay elapsed — runner plumbing, not a command failure.
func TestBatchTreatsBareErrWaitDelayAsSuccess(t *testing.T) {
	r := &recordingRunner{streamErr: exec.ErrWaitDelay}
	c := Console{R: r}
	_, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "x", Kind: updplan.KindSync}, "script")
	if err != nil {
		t.Fatalf("a bare exec.ErrWaitDelay (child exited 0) must be treated as success: %v", err)
	}
}

// --- leaf D+E findings ------------------------------------------------------
//
// The tests below pin the final round of code-review findings on the
// fleet-update build's D+E leaves (docs/mbo/plans/fleet-update.md).

// --- finding 1: Preamble is prepended verbatim, no separator added ----------

// TestBackgroundRunStepScriptIsValidShell proves the assembled preamble +
// script is syntactically valid POSIX shell (via `sh -n`, a stub-free
// syntax check) for a preamble containing the sudo prime + sudoGate +
// exported answers — the exact shape bgPreamble produces. Before the fix,
// Console.runScript joined Preamble's text with " && ", producing
// "... || exit 92;  && cd ..." — a syntax error on EVERY TUI background run
// step, since bgPreamble already terminates its own text with "; ".
func TestBackgroundRunStepScriptIsValidShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -n requires a POSIX shell")
	}
	preamble := `sudo -S -p '' -v 2>/dev/null || exit 91; ` +
		`{ [ "$(id -u)" = 0 ] || ! command -v sudo >/dev/null 2>&1 || sudo -n true 2>/dev/null; } || exit 92; ` +
		`export WINSETUP_ANSWER=s GEMINI_TEARDOWN_ANSWER=keep; `
	c := Console{Preamble: func(updplan.Step) string { return preamble }}
	st := updplan.Step{ID: "install", Kind: updplan.KindRun}
	script := c.runScript(st, "cd /tmp && true")

	if err := exec.Command("sh", "-n", "-c", script).Run(); err != nil {
		t.Fatalf("assembled script is not valid shell: %v\nscript:\n%s", err, script)
	}
}

// --- finding 5: HostReport.NeedsTerminal must use a typed marker, not a string match

// TestGhAuthNeedingATerminalIsFlaggedForTheInteractiveLane pins the fix: a
// gh-auth login that fails with ErrNoTerminal (the Background lane's shape)
// must set Result.NeedsTerminal so HostReport.NeedsTerminal() finds it —
// before the fix, runGhAuth rewrote Reason to the human-readable "needs a
// terminal", which no longer matched ErrNoTerminal.Error(), so the TUI never
// routed such a host to its interactive queue.
func TestGhAuthNeedingATerminalIsFlaggedForTheInteractiveLane(t *testing.T) {
	inner := newFakeIO().on("gh auth status -h github.com", "", realExitError(t, 1))
	io := noTerminalIO{inner}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", ghAuthPlan())

	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != Failed || r.Reason != "needs a terminal" {
		t.Fatalf("no-terminal login must still fail cleanly with the human reason: %+v", r)
	}
	if !r.NeedsTerminal {
		t.Fatalf("Result.NeedsTerminal must be set: %+v", r)
	}
	if !rep.NeedsTerminal() {
		t.Fatal("HostReport.NeedsTerminal() must report true for a gh-auth login needing a terminal")
	}
}

// --- finding 7: the executor tees every lane's remote output into the capture

// TestExecutorTeesBatchOutputIntoTheCapture proves RunHost forwards every
// line a StepIO lane produces into the host's own capture (Output/
// LineWriter), even when the caller never set Console.Line itself — a
// headless `fleet update` run used to capture only the executor's own step
// banners, never the remote command's own output (e.g. git's `fatal:` text).
func TestExecutorTeesBatchOutputIntoTheCapture(t *testing.T) {
	out := newRecordingOutput()
	r := &recordingRunner{streamOut: "line one\nline two"}
	e := Executor{IO: Console{R: r}, Out: out}
	e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))

	text := out.text("alpha")
	for _, want := range []string{"line one", "line two"} {
		if !strings.Contains(text, want) {
			t.Fatalf("capture must contain the lane's own remote output %q, got:\n%s", want, text)
		}
	}
}

// TestExecutorTeeingPreservesAnExistingLineCallback proves the tee ADDS to
// whatever Line callback the caller already set (the TUI's log-pane feed)
// rather than replacing it.
func TestExecutorTeeingPreservesAnExistingLineCallback(t *testing.T) {
	out := newRecordingOutput()
	r := &recordingRunner{streamOut: "line one"}
	var seen []string
	e := Executor{
		IO:  Console{R: r, Line: func(_, l string) { seen = append(seen, l) }},
		Out: out,
	}
	e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))

	if len(seen) == 0 || seen[0] != "line one" {
		t.Fatalf("the caller's own Line callback must still fire: %v", seen)
	}
	if !strings.Contains(out.text("alpha"), "line one") {
		t.Fatalf("the capture must ALSO receive the line:\n%s", out.text("alpha"))
	}
}
