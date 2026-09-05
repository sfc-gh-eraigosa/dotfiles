package updexec

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

var errDeviceFlowDenied = errors.New("device flow denied")

// realExitError returns an authentic *exec.ExitError with the given exit
// code, by actually running a tiny local command — exitCode()/classify()
// are defined in terms of *exec.ExitError, so a test needs the real type,
// not a hand-rolled stand-in.
func realExitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
	if err == nil {
		t.Fatalf("exit %d unexpectedly succeeded", code)
	}
	return err
}

// --- test doubles ------------------------------------------------------------

type resp struct {
	out string
	err error
}

type recordedCall struct {
	lane   string // "batch" | "interactive"
	host   string
	step   string
	kind   updplan.Kind
	script string
}

// fakeIO is a scripted StepIO double, keyed by a SUBSTRING of the script
// (never the step id alone — a single sync step sends several distinct
// scripts: precheck, then clone/rescue/sync). Each key holds an ORDERED
// queue of responses, consumed FIFO, so a test can script "fail twice then
// succeed". A key registered via block() hangs the call on ctx.Done()
// instead, to drive the timeout/cancellation paths deterministically.
type fakeIO struct {
	mu          sync.Mutex
	batch       map[string][]resp
	order       []string // batch keys in registration order (tie-break for pick)
	interactive map[string][]error
	block       map[string]bool
	calls       []recordedCall
}

func newFakeIO() *fakeIO {
	return &fakeIO{
		batch:       map[string][]resp{},
		interactive: map[string][]error{},
		block:       map[string]bool{},
	}
}

func (f *fakeIO) on(substr, out string, err error) *fakeIO {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, seen := f.batch[substr]; !seen {
		f.order = append(f.order, substr)
	}
	f.batch[substr] = append(f.batch[substr], resp{out, err})
	return f
}

func (f *fakeIO) onInteractive(substr string, err error) *fakeIO {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interactive[substr] = append(f.interactive[substr], err)
	return f
}

func (f *fakeIO) blockOn(substr string) *fakeIO {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.block[substr] = true
	return f
}

func (f *fakeIO) Batch(ctx context.Context, host string, st updplan.Step, script string) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{"batch", host, st.ID, st.Kind, script})
	var r resp
	blocked := false
	for key := range f.block {
		if strings.Contains(script, key) {
			blocked = true
			break
		}
	}
	if !blocked {
		// Deterministic match: the LONGEST matching key wins, registration
		// order breaks ties. Iterating the map directly made the choice
		// random whenever two keys matched one script (the rescue script
		// carries the sync prologue's `orig=$(git symbolic-ref` text), which
		// flaked TestRescueOffBranchRestoresTheBranchWithoutAStash ~15 %.
		if key, ok := f.pick(script); ok {
			q := f.batch[key]
			r = q[0]
			f.batch[key] = q[1:]
		}
	}
	f.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return r.out, r.err
}

// pick returns the most specific registered batch key that matches script and
// still has a queued response. Callers hold f.mu.
func (f *fakeIO) pick(script string) (string, bool) {
	best, found := "", false
	for _, key := range f.order {
		if q := f.batch[key]; len(q) > 0 && strings.Contains(script, key) && (!found || len(key) > len(best)) {
			best, found = key, true
		}
	}
	return best, found
}

func (f *fakeIO) Interactive(ctx context.Context, host string, st updplan.Step, script string) error {
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{"interactive", host, st.ID, st.Kind, script})
	var err error
	blocked := false
	for key := range f.block {
		if strings.Contains(script, key) {
			blocked = true
			break
		}
	}
	if !blocked {
		for key, q := range f.interactive {
			if strings.Contains(script, key) && len(q) > 0 {
				err = q[0]
				f.interactive[key] = q[1:]
				break
			}
		}
	}
	f.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

func (f *fakeIO) batchCalls() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedCall
	for _, c := range f.calls {
		if c.lane == "batch" {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeIO) interactiveCalls() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedCall
	for _, c := range f.calls {
		if c.lane == "interactive" {
			out = append(out, c)
		}
	}
	return out
}

// noTerminalIO wraps a fakeIO and always fails Interactive with
// ErrNoTerminal — the Background lane's shape, without pulling runner in.
type noTerminalIO struct{ *fakeIO }

func (n noTerminalIO) Interactive(context.Context, string, updplan.Step, string) error {
	return ErrNoTerminal
}

// stepClock is an injectable, deterministic Now(): each call advances by
// step.
type stepClock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.t
	c.t = c.t.Add(c.step)
	return now
}

// sleepRecorder records every backoff wait instead of actually sleeping.
type sleepRecorder struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (s *sleepRecorder) Sleep(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waits = append(s.waits, d)
}

func (s *sleepRecorder) all() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

// midpointRand is Backoff.Wait's nil-equivalent (0.5, no jitter movement),
// made explicit so tests assert exact schedules.
func midpointRand() float64 { return 0.5 }

// recordingOutput captures every Line/Close call per host, so a test can
// assert on the capture text without a real file.
type recordingOutput struct {
	mu   sync.Mutex
	logs map[string]*[]string
}

func newRecordingOutput() *recordingOutput { return &recordingOutput{logs: map[string]*[]string{}} }

type recordingWriter struct {
	out *recordingOutput
	buf *[]string
}

func (o *recordingOutput) Open(host, header string) (LineWriter, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	buf := &[]string{header}
	o.logs[host] = buf
	return recordingWriter{o, buf}, "log:" + host
}

func (w recordingWriter) Line(s string) {
	w.out.mu.Lock()
	defer w.out.mu.Unlock()
	*w.buf = append(*w.buf, s)
}

func (w recordingWriter) Close(footer string) {
	w.out.mu.Lock()
	defer w.out.mu.Unlock()
	*w.buf = append(*w.buf, footer)
}

func (o *recordingOutput) text(host string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	buf := o.logs[host]
	if buf == nil {
		return ""
	}
	return strings.Join(*buf, "\n")
}

// --- plan builders -------------------------------------------------------

func stdRetry() updplan.Retry {
	return updplan.Retry{
		Attempts: 1,
		On:       []updplan.RetryOn{updplan.RetryOnTransport},
		Backoff:  updplan.Backoff{Initial: 5 * time.Second, Max: 2 * time.Minute, Factor: 2, Jitter: true},
	}
}

func stdExpect() updplan.Expect { return updplan.Expect{Exit: []int{0}} }

// syncRepo returns a repo suitable for direct use in a hand-built Plan
// (skip the resolveRepoPath step: give it an already-remote path).
func syncRepo(name string, local updplan.Local) updplan.Repo {
	return updplan.Repo{
		Name:     name,
		Path:     "~/git/" + name,
		Branches: []string{"main"},
		Local:    local,
		Restore:  true,
	}
}

func syncStep(id, repo string) updplan.Step {
	return updplan.Step{
		ID: id, Kind: updplan.KindSync, Repo: repo,
		Expect: stdExpect(), OnFailure: updplan.OnFailureStop, Retry: stdRetry(),
	}
}

func runStep(id, repo, run string, needs ...string) updplan.Step {
	return updplan.Step{
		ID: id, Kind: updplan.KindRun, Repo: repo, Run: run, Needs: needs,
		Expect: stdExpect(), OnFailure: updplan.OnFailureStop, Retry: stdRetry(),
	}
}

func interactiveRunStep(id, repo, run string, needs ...string) updplan.Step {
	st := runStep(id, repo, run, needs...)
	st.Interactive = true
	st.Retry = updplan.Retry{Attempts: 1}
	return st
}

// defaultTestPlan mirrors updplan.Default(): dotfiles.sync -> dotfiles.install.
func defaultTestPlan() updplan.Plan {
	return updplan.Plan{
		Root: "~/git",
		Repos: map[string]updplan.Repo{
			"dotfiles": syncRepo("dotfiles", updplan.LocalSkip),
		},
		Steps: []updplan.Step{
			syncStep("dotfiles.sync", "dotfiles"),
			interactiveRunStep("dotfiles.install", "dotfiles", "./install.sh", "dotfiles.sync"),
		},
		Source: "built-in",
	}
}

const cleanPrecheck = "state=clean branch=main"

// --- task 10: executor walks the plan per host ------------------------------

func TestRunHostRunsStepsInOrderWithDurations(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	io.onInteractive("install.sh", nil)

	clock := &stepClock{t: time.Unix(1000, 0), step: 5 * time.Second}
	out := newRecordingOutput()
	e := Executor{IO: io, Out: out, Now: clock.Now}

	rep := e.RunHost("alpha", defaultTestPlan())

	if len(rep.Results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(rep.Results), rep.Results)
	}
	if rep.Results[0].Step != "dotfiles.sync" || rep.Results[1].Step != "dotfiles.install" {
		t.Fatalf("steps out of order: %+v", rep.Results)
	}
	for _, r := range rep.Results {
		if r.Status != OK {
			t.Fatalf("step %s not ok: %+v", r.Step, r)
		}
		if r.Duration <= 0 {
			t.Fatalf("step %s duration not measured: %+v", r.Step, r)
		}
	}
	if rep.Output == "" {
		t.Fatal("HostReport.Output must carry the capture path")
	}
	text := out.text("alpha")
	if !strings.Contains(text, "=== step dotfiles.sync (sync) ===") {
		t.Fatalf("capture missing sync step header:\n%s", text)
	}
}

func TestNotesAreParsedFromFleetLines(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: switched main -> main\nnot-a-note", nil)
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalSkip)},
		Steps: []updplan.Step{syncStep("dotfiles.sync", "dotfiles")},
	}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)
	got := rep.Results[0].Notes
	want := []string{"orig=main", "switched main -> main"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Notes = %v, want %v", got, want)
	}
}

func TestAttemptHeaderIsWrittenToTheCapture(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=missing", nil)
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalSkip)},
		Steps: []updplan.Step{syncStep("dotfiles.sync", "dotfiles")},
	}
	out := newRecordingOutput()
	e := Executor{IO: io, Out: out}
	e.RunHost("alpha", p)
	text := out.text("alpha")
	if !strings.Contains(text, "=== step dotfiles.sync (sync) ===") {
		t.Fatalf("capture missing step header:\n%s", text)
	}
}

// --- task 11: failure cascade ------------------------------------------------

func resultFor(rep HostReport, id string) (Result, bool) {
	for _, r := range rep.Results {
		if r.Step == id {
			return r, true
		}
	}
	return Result{}, false
}

// a: sync fails -> b (needs a, on_failure stop) is blocked -> independent
// dotfiles.* steps still run.
func TestFailedStepSkipsTransitiveDependents(t *testing.T) {
	io := newFakeIO().
		on("cd ~/git/dotfiles && g=$(git rev-parse", "state=missing", nil). // dotfiles.sync: missing, no url -> failed
		on("cd ~/git/scripts && g=$(git rev-parse", cleanPrecheck, nil).    // scripts.sync: clean -> ok
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil).             // scripts.sync's own sync body
		on("make", "", nil)

	p := updplan.Plan{
		Repos: map[string]updplan.Repo{
			"dotfiles": syncRepo("dotfiles", updplan.LocalSkip),
			"scripts":  syncRepo("scripts", updplan.LocalSkip),
		},
		Steps: []updplan.Step{
			syncStep("dotfiles.sync", "dotfiles"),
			runStep("scripts.make", "scripts", "make", "dotfiles.sync"),
			syncStep("scripts.sync", "scripts"),
		},
	}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)

	blocked, ok := resultFor(rep, "scripts.make")
	if !ok || blocked.Status != DepFailed || blocked.Reason != "blocked by dotfiles.sync" {
		t.Fatalf("scripts.make = %+v, want dependency-failed blocked by dotfiles.sync", blocked)
	}
	indep, ok := resultFor(rep, "scripts.sync")
	if !ok || indep.Status != OK {
		t.Fatalf("scripts.sync (independent) = %+v, want ok", indep)
	}
}

func TestOnFailureContinueLetsDependentsRunButStillFailsTheHost(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=missing", nil).
		on("make", "", nil)
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalSkip)},
		Steps: []updplan.Step{
			func() updplan.Step {
				s := syncStep("dotfiles.sync", "dotfiles")
				s.OnFailure = updplan.OnFailureContinue
				return s
			}(),
			runStep("scripts.make", "", "make", "dotfiles.sync"),
		},
	}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)

	dep, ok := resultFor(rep, "scripts.make")
	if !ok || dep.Status != OK {
		t.Fatalf("dependent under on_failure: continue should still run: %+v", dep)
	}
	if !rep.Failed() {
		t.Fatal("host must still be reported failed overall")
	}
}

func TestExpectExitAcceptsNonZero(t *testing.T) {
	fio := newFakeIO()
	st := runStep("x", "", "echo hi")
	st.Expect = updplan.Expect{Exit: []int{3}}
	p := updplan.Plan{Steps: []updplan.Step{st}}
	fio.on("echo hi", "", realExitError(t, 3))
	e := Executor{IO: fio}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "x")
	if !ok || r.Status != OK || r.Exit != 3 {
		t.Fatalf("expect.exit=[3] with exit 3 should be ok: %+v", r)
	}
}

func TestDependencyFailedAlsoBlocks(t *testing.T) {
	io := newFakeIO().on("git rev-parse --git-dir", "state=missing", nil).on("echo b", "", nil).on("echo c", "", nil)
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalSkip)},
		Steps: []updplan.Step{
			syncStep("a", "dotfiles"),
			runStep("b", "", "echo b", "a"),
			runStep("c", "", "echo c", "b"),
		},
	}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)
	c, ok := resultFor(rep, "c")
	if !ok || c.Status != DepFailed || c.Reason != "blocked by b" {
		t.Fatalf("c = %+v, want dependency-failed blocked by b", c)
	}
}

// --- task 12: local-state policies -------------------------------------------

// onlySyncPlan builds a one-repo, one-step plan for the sync-decision tests.
func onlySyncPlan(local updplan.Local, withURL bool) updplan.Plan {
	r := syncRepo("dotfiles", local)
	if withURL {
		r.URL = "https://example.com/dotfiles.git"
	}
	return updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": r},
		Steps: []updplan.Step{syncStep("dotfiles.sync", "dotfiles")},
	}
}

// TestUpdateSkipsDirtyCloneByDefault is migrated from cmd/update_test.go: a
// dirty clone under local: skip is skipped, and NOTHING beyond the
// (read-only) precheck is ever sent.
func TestUpdateSkipsDirtyCloneByDefault(t *testing.T) {
	io := newFakeIO().on("git rev-parse --git-dir", "state=dirty branch=main", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != Skipped {
		t.Fatalf("dirty clone under local: skip must be skipped: %+v", r)
	}
	if r.Reason == "" {
		t.Fatal("a skip must state a reason")
	}
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, "install.sh") || strings.Contains(c.script, "git merge") {
			t.Fatalf("a skipped host must not be mutated: %q", c.script)
		}
	}
}

func TestUpdateProceedsOnCleanClone(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("a clean clone must sync: %+v", r)
	}
}

// TestForceRescuesDirtyWorkBeforePulling: --force (local: rescue) preserves
// dirty work in a rescue worktree, then syncs — never a hard reset.
func TestForceRescuesDirtyWorkBeforePulling(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("fleet-rescue/$ts", "", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalRescue, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("rescue then sync must succeed: %+v", r)
	}
	calls := io.batchCalls()
	if len(calls) != 3 {
		t.Fatalf("want precheck+rescue+sync = 3 batch calls, got %d: %+v", len(calls), calls)
	}
	for _, c := range calls {
		if strings.Contains(c.script, "reset --hard") || strings.Contains(c.script, "checkout -- ") {
			t.Fatalf("--force must never discard local work: %q", c.script)
		}
	}
	joined := calls[1].script
	for _, want := range []string{"git add -A", "worktree add"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("--force must preserve work via %q: %q", want, joined)
		}
	}
	if !strings.Contains(calls[1].script, "fleet-rescue") || !strings.Contains(calls[2].script, "orig=$(git symbolic-ref") {
		t.Fatalf("rescue must run BEFORE sync: %+v", calls)
	}
}

func TestUpdateSurfacesProbeFailure(t *testing.T) {
	io := newFakeIO().on("git rev-parse --git-dir", "", ErrTransport)
	e := Executor{IO: io}
	rep := e.RunHost("dead", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != Failed {
		t.Fatalf("an unreachable host must surface a failure: %+v", r)
	}
}

func TestMissingCloneWithURLClones(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=missing", nil).
		on("git clone -q", "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, true))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("missing + url must clone and succeed: %+v", r)
	}
}

func TestMissingCloneWithoutURLFails(t *testing.T) {
	io := newFakeIO().on("git rev-parse --git-dir", "state=missing", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != Failed {
		t.Fatalf("missing without url must fail: %+v", r)
	}
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, "git clone") {
			t.Fatal("must never clone without a url")
		}
	}
}

func TestInProgressMergeIsSkippedUnderEveryPolicy(t *testing.T) {
	for _, local := range []updplan.Local{updplan.LocalSkip, updplan.LocalRescue, updplan.LocalCarry} {
		t.Run(string(local), func(t *testing.T) {
			io := newFakeIO().on("git rev-parse --git-dir", "state=in-progress branch=main", nil)
			e := Executor{IO: io}
			rep := e.RunHost("alpha", onlySyncPlan(local, false))
			r, ok := resultFor(rep, "dotfiles.sync")
			if !ok || r.Status != Skipped {
				t.Fatalf("in-progress under %s must be skipped: %+v", local, r)
			}
		})
	}
}

func TestResetModeUsesResetScript(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	e := Executor{IO: io, Reset: true}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("reset sync must succeed: %+v", r)
	}
	found := false
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, "git reset --hard FETCH_HEAD") {
			found = true
		}
		if strings.Contains(c.script, "orig=$(git symbolic-ref") && !strings.Contains(c.script, "git reset --hard FETCH_HEAD") {
			t.Fatalf("--reset must replace the merge with ResetScript: %q", c.script)
		}
	}
	if !found {
		t.Fatal("--reset must use ResetScript's git reset --hard FETCH_HEAD")
	}
}

func TestUnexpectedPrecheckOutputFails(t *testing.T) {
	io := newFakeIO().on("git rev-parse --git-dir", "garbage output", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != Failed {
		t.Fatalf("unexpected precheck output must fail: %+v", r)
	}
}

func TestCLILocalOverridesEveryRepoPolicy(t *testing.T) {
	// The repo says skip, but Executor.Local=rescue must win.
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("fleet-rescue/$ts", "", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	e := Executor{IO: io, Local: updplan.LocalRescue}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("Executor.Local must override the repo's own policy: %+v", r)
	}
}

func TestResetIsIncompatibleWithCarry(t *testing.T) {
	io := newFakeIO()
	e := Executor{IO: io, Reset: true}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalCarry, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != Failed || r.Reason != "--reset is incompatible with local: carry" {
		t.Fatalf("reset+carry must fail up front with the exact reason: %+v", r)
	}
	if len(io.batchCalls()) != 0 {
		t.Fatal("reset+carry must be rejected before any remote call")
	}
}

// --- task 13: carry and branch restore ---------------------------------------

const restoreFailedMarker = "restore-failed stash="

func TestCarryStashesWithUntrackedAndCapturesTheSHA(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash="+sha+" from=main", nil).
		on(restoreFailedMarker, "", nil) // never actually reached (nothing armed beyond stash+same branch -> IS armed, restore WILL run)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalCarry, false))
	r, ok := resultFor(rep, "dotfiles.sync")
	if !ok || r.Status != OK {
		t.Fatalf("carry sync must succeed: %+v", r)
	}
	found := false
	for _, n := range r.Notes {
		if strings.Contains(n, "carried stash="+sha) {
			found = true
		}
	}
	if !found {
		t.Fatalf("Notes must capture the carried stash SHA: %+v", r.Notes)
	}
}

func TestCarryRestoreRunsAfterTheLastStepUsingTheRepo(t *testing.T) {
	io := newFakeIO().
		on("cd ~/git/r && g=$(git rev-parse", "state=dirty branch=main", nil).
		on("cd ~/git/other && g=$(git rev-parse", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa from=main", nil).
		on("cd ~/git/r/scripts && make", "", nil).
		on(restoreFailedMarker, "", nil)
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{
			"r":     syncRepo("r", updplan.LocalCarry),
			"other": syncRepo("other", updplan.LocalSkip),
		},
		Steps: []updplan.Step{
			syncStep("r.sync", "r"),
			func() updplan.Step {
				s := runStep("r.build", "r", "make", "r.sync")
				return s
			}(),
			syncStep("other.sync", "other"),
		},
	}
	// r.build's script is "cd ~/git/r && make" per RunScript; register it too.
	io.on("cd ~/git/r && make", "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)

	var order []string
	for _, c := range io.batchCalls() {
		order = append(order, c.script)
	}
	buildIdx, restoreIdx, otherIdx := -1, -1, -1
	for i, s := range order {
		if strings.Contains(s, "cd ~/git/r && make") && buildIdx == -1 {
			buildIdx = i
		}
		if strings.Contains(s, restoreFailedMarker) && restoreIdx == -1 {
			restoreIdx = i
		}
		if strings.Contains(s, "cd ~/git/other && g=$(git rev-parse") && otherIdx == -1 {
			otherIdx = i
		}
	}
	if buildIdx == -1 || restoreIdx == -1 || otherIdx == -1 {
		t.Fatalf("missing expected calls: build=%d restore=%d other=%d, order=%v", buildIdx, restoreIdx, otherIdx, order)
	}
	if buildIdx >= restoreIdx || restoreIdx >= otherIdx {
		t.Fatalf("restore must run after r.build and before other.sync: build=%d restore=%d other=%d", buildIdx, restoreIdx, otherIdx)
	}
	if rr, ok := resultFor(rep, "r.restore"); !ok || rr.Status != OK {
		t.Fatalf("r.restore = %+v, want ok", rr)
	}
}

func TestRestoreRunsEvenWhenAnIntermediateStepFailed(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa from=main", nil).
		on(restoreFailedMarker, "", nil).
		on("echo unrelated", "", realExitError(t, 1))
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalCarry)},
		Steps: []updplan.Step{
			syncStep("dotfiles.sync", "dotfiles"),
			func() updplan.Step {
				s := runStep("unrelated", "", "echo unrelated")
				s.OnFailure = updplan.OnFailureContinue
				return s
			}(),
			runStep("dotfiles.build", "dotfiles", "./build.sh", "dotfiles.sync"),
		},
	}
	io.on("./build.sh", "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)

	if u, ok := resultFor(rep, "unrelated"); !ok || u.Status != Failed {
		t.Fatalf("unrelated = %+v, want failed", u)
	}
	if rr, ok := resultFor(rep, "dotfiles.restore"); !ok || rr.Status != OK {
		t.Fatalf("restore must still run despite the unrelated failure: %+v", rr)
	}
}

func TestRestoreRunsImmediatelyWhenSyncFailsAfterStash(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa from=main", realExitError(t, 1)).
		on(restoreFailedMarker, "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalCarry, false))

	sync, ok := resultFor(rep, "dotfiles.sync")
	if !ok || sync.Status != Failed {
		t.Fatalf("sync = %+v, want failed", sync)
	}
	// The restore must be the VERY NEXT result, not deferred.
	if len(rep.Results) < 2 || rep.Results[1].Step != "dotfiles.restore" {
		t.Fatalf("restore did not run immediately after the failed sync: %+v", rep.Results)
	}
}

func TestRestoreConflictKeepsTheStash(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash="+sha+" from=main", nil).
		on(restoreFailedMarker, "fleet: restore-failed stash="+sha+" branch=main", realExitError(t, 4))
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalCarry, false))

	rr, ok := resultFor(rep, "dotfiles.restore")
	if !ok || rr.Status != Failed {
		t.Fatalf("restore = %+v, want failed", rr)
	}
	if !strings.Contains(rr.Reason, sha) || !strings.Contains(rr.Reason, "main") {
		t.Fatalf("reason must name the SHA and branch: %q", rr.Reason)
	}
}

func TestCleanOffBranchIsRestoredUnderEveryPolicy(t *testing.T) {
	for _, local := range []updplan.Local{updplan.LocalSkip, updplan.LocalRescue, updplan.LocalCarry} {
		t.Run(string(local), func(t *testing.T) {
			io := newFakeIO().
				on("git rev-parse --git-dir", "state=clean branch=feature", nil).
				on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil).
				on(restoreFailedMarker, "", nil)
			e := Executor{IO: io}
			rep := e.RunHost("alpha", onlySyncPlan(local, false))
			rr, ok := resultFor(rep, "dotfiles.restore")
			if !ok || rr.Status != OK {
				t.Fatalf("%s: off-branch clean sync must be restored: %+v", local, rr)
			}
		})
	}
}

func TestOnTargetNeverSynthesizesARestore(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	if _, ok := resultFor(rep, "dotfiles.restore"); ok {
		t.Fatal("a sync that never switched or stashed must never synthesize a restore step")
	}
}

func TestRescueOffBranchRestoresTheBranchWithoutAStash(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=feature", nil).
		on("fleet-rescue/$ts", "", nil).
		// Keyed on the fetch, not the prologue: the rescue script carries the
		// same `orig=$(git symbolic-ref` text, so that key would be ambiguous.
		on("git fetch origin", "fleet: orig=feature\nfleet: switched feature -> main", nil).
		on(restoreFailedMarker, "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalRescue, false))
	rr, ok := resultFor(rep, "dotfiles.restore")
	if !ok || rr.Status != OK {
		t.Fatalf("rescue off-branch must still be restored: %+v", rr)
	}
	// Find the restore call and confirm it carried no stash (empty sha).
	calls := io.batchCalls()
	var restoreScript string
	for _, c := range calls {
		if strings.Contains(c.script, restoreFailedMarker) {
			restoreScript = c.script
		}
	}
	if restoreScript == "" {
		t.Fatal("no restore call recorded")
	}
	// The stash-apply clause is always present in the script text (it is a
	// conditional), but with an empty sha the `[ -z "" ]` guard is true, so
	// it never actually runs.
	if !strings.Contains(restoreScript, `[ -z "" ]`) {
		t.Fatalf("a rescue-only restore (no carried stash) must guard the stash apply on an empty sha: %q", restoreScript)
	}
}

func TestDetachedHeadRestoresToTheSHA(t *testing.T) {
	sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=clean branch=detached", nil).
		on("orig=$(git symbolic-ref", "fleet: orig="+sha+"\nfleet: switched "+sha+" -> main", nil).
		on(restoreFailedMarker, "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	rr, ok := resultFor(rep, "dotfiles.restore")
	if !ok || rr.Status != OK {
		t.Fatalf("detached HEAD must be restored to the SHA: %+v", rr)
	}
	var restoreScript string
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, restoreFailedMarker) {
			restoreScript = c.script
		}
	}
	if !strings.Contains(restoreScript, "git checkout -q "+sha) {
		t.Fatalf("restore must check out the exact SHA: %q", restoreScript)
	}
}

// TestRestoreFalseLeavesHostOnTarget is corrected per the leaf B code
// review: a synthesized "<repo>.restore" Result — even Skipped — is still
// non-ok, and HostReport.Failed() treats ANY non-ok Result as a host
// failure. That made every successful run that switched branches with
// restore disabled report a FAILED host, which is the opposite of what
// "disabled" should mean. The fix emits NO restore Result at all when
// restore is disabled; the fact lives in a note on the sync step instead.
func TestRestoreFalseLeavesHostOnTarget(t *testing.T) {
	base := func() *fakeIO {
		return newFakeIO().
			on("git rev-parse --git-dir", "state=clean branch=feature", nil).
			on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil)
	}

	t.Run("repo.Restore=false", func(t *testing.T) {
		io := base()
		e := Executor{IO: io}
		p := onlySyncPlan(updplan.LocalSkip, false)
		r := p.Repos["dotfiles"]
		r.Restore = false
		p.Repos["dotfiles"] = r
		rep := e.RunHost("alpha", p)
		if rep.Failed() {
			t.Fatalf("a successful sync with restore disabled must not fail the host: %+v", rep.Results)
		}
		if _, ok := resultFor(rep, "dotfiles.restore"); ok {
			t.Fatal("restore: false must never synthesize a <repo>.restore result")
		}
		for _, c := range io.batchCalls() {
			if strings.Contains(c.script, "git checkout -q feature") {
				t.Fatal("restore: false must never send a restore script")
			}
		}
	})

	t.Run("Executor.NoRestore", func(t *testing.T) {
		io := base()
		e := Executor{IO: io, NoRestore: true}
		rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
		if rep.Failed() {
			t.Fatalf("--no-restore on a successful sync must not fail the host: %+v", rep.Results)
		}
		if _, ok := resultFor(rep, "dotfiles.restore"); ok {
			t.Fatal("--no-restore must never synthesize a <repo>.restore result")
		}
	})
}

// TestDisabledRestoreDoesNotFailTheHost is the direct assertion the finding
// asked for: a plan whose repo switched branches, with restore disabled,
// must report a fully successful HostReport.
func TestDisabledRestoreDoesNotFailTheHost(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=clean branch=feature", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil)
	e := Executor{IO: io, NoRestore: true}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	if rep.Failed() {
		t.Fatalf("HostReport.Failed() = true, want false: %+v", rep.Results)
	}
	sync, ok := resultFor(rep, "dotfiles.sync")
	if !ok || sync.Status != OK {
		t.Fatalf("sync = %+v, want ok", sync)
	}
	found := false
	for _, n := range sync.Notes {
		if strings.Contains(n, "restore disabled") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sync step must note that restore was disabled: %+v", sync.Notes)
	}
}

func TestRestoreCheckoutFailureKeepsEverything(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=clean branch=feature", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil).
		on(restoreFailedMarker, "fleet: restore-failed stash= branch=feature", realExitError(t, 4))
	e := Executor{IO: io}
	rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
	rr, ok := resultFor(rep, "dotfiles.restore")
	if !ok || rr.Status != Failed {
		t.Fatalf("a failed checkout must be reported failed: %+v", rr)
	}
	if !strings.Contains(rr.Reason, "feature") {
		t.Fatalf("reason must name the branch: %q", rr.Reason)
	}
}

func TestRestoreStepHasFixedRetryPolicy(t *testing.T) {
	t.Run("retries transport failures 3x regardless of NoRetry", func(t *testing.T) {
		io := newFakeIO().
			on("git rev-parse --git-dir", "state=clean branch=feature", nil).
			on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil).
			on(restoreFailedMarker, "", ErrTransport).
			on(restoreFailedMarker, "", ErrTransport).
			on(restoreFailedMarker, "", nil)
		sleeper := &sleepRecorder{}
		e := Executor{IO: io, NoRetry: true, Sleep: sleeper.Sleep, Rand: midpointRand}
		rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
		rr, ok := resultFor(rep, "dotfiles.restore")
		if !ok || rr.Status != OK || rr.Attempts != 3 {
			t.Fatalf("restore must retry transport failures up to 3 attempts despite NoRetry: %+v", rr)
		}
	})

	t.Run("a conflict is not retried", func(t *testing.T) {
		io := newFakeIO().
			on("git rev-parse --git-dir", "state=clean branch=feature", nil).
			on("orig=$(git symbolic-ref", "fleet: orig=feature\nfleet: switched feature -> main", nil).
			on(restoreFailedMarker, "fleet: restore-failed stash= branch=feature", realExitError(t, 4))
		e := Executor{IO: io}
		rep := e.RunHost("alpha", onlySyncPlan(updplan.LocalSkip, false))
		rr, ok := resultFor(rep, "dotfiles.restore")
		if !ok || rr.Status != Failed || rr.Attempts != 1 {
			t.Fatalf("a non-transport restore failure must not be retried: %+v", rr)
		}
	})
}

// --- task 14: gh-auth step ----------------------------------------------------

func ghAuthPlan() updplan.Plan {
	return updplan.Plan{
		Steps: []updplan.Step{
			{ID: "gh.auth", Kind: updplan.KindGhAuth, Hostname: "github.com",
				Expect: stdExpect(), OnFailure: updplan.OnFailureStop, Retry: stdRetry()},
		},
	}
}

func TestGhAuthSkipsLoginWhenStatusPasses(t *testing.T) {
	io := newFakeIO().on("gh auth status -h github.com", "", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", ghAuthPlan())
	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != OK {
		t.Fatalf("an authenticated host must pass without logging in: %+v", r)
	}
	if len(io.batchCalls()) != 1 {
		t.Fatalf("want exactly 1 Batch call, got %d: %+v", len(io.batchCalls()), io.batchCalls())
	}
	if len(io.interactiveCalls()) != 0 {
		t.Fatalf("want 0 Interactive calls, got %d: %+v", len(io.interactiveCalls()), io.interactiveCalls())
	}
}

func TestGhAuthLogsInInteractivelyThenReverifies(t *testing.T) {
	io := newFakeIO().
		on("gh auth status -h github.com", "", realExitError(t, 1)).
		on("gh auth status -h github.com", "", nil).
		onInteractive("gh auth login -h github.com", nil)
	e := Executor{IO: io}
	rep := e.RunHost("alpha", ghAuthPlan())
	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != OK {
		t.Fatalf("login then re-check must succeed: %+v", r)
	}
	if len(io.batchCalls()) != 2 {
		t.Fatalf("want 2 Batch calls (check, recheck), got %d: %+v", len(io.batchCalls()), io.batchCalls())
	}
	if len(io.interactiveCalls()) != 1 {
		t.Fatalf("want exactly 1 Interactive (login) call, got %d", len(io.interactiveCalls()))
	}
}

func TestGhAuthReports127AsNotInstalled(t *testing.T) {
	io := newFakeIO().on("gh auth status -h github.com", "", realExitError(t, 127))
	e := Executor{IO: io}
	rep := e.RunHost("alpha", ghAuthPlan())
	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != Failed || r.Reason != "gh not installed" {
		t.Fatalf("exit 127 must be reported as gh not installed: %+v", r)
	}
	if len(io.interactiveCalls()) != 0 {
		t.Fatal("a missing gh must never attempt an interactive login")
	}
}

func TestGhAuthWithoutATerminalFailsCleanly(t *testing.T) {
	inner := newFakeIO().on("gh auth status -h github.com", "", realExitError(t, 1))
	io := noTerminalIO{inner}
	p := ghAuthPlan()
	p.Steps = append(p.Steps, runStep("after", "", "echo hi", "gh.auth"))
	e := Executor{IO: io}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != Failed || r.Reason != "needs a terminal" {
		t.Fatalf("no-terminal login must fail cleanly: %+v", r)
	}
	after, ok := resultFor(rep, "after")
	if !ok || after.Status != DepFailed {
		t.Fatalf("a dependent must be blocked: %+v", after)
	}
}

func TestGhAuthNeverUsesStdin(t *testing.T) {
	io := newFakeIO().
		on("gh auth status -h github.com", "", realExitError(t, 1)).
		on("gh auth status -h github.com", "", nil).
		onInteractive("gh auth login -h github.com", nil)
	e := Executor{IO: io}
	e.RunHost("alpha", ghAuthPlan())
	for _, c := range io.calls {
		if strings.Contains(c.script, "GH_TOKEN") || strings.Contains(c.script, "GITHUB_TOKEN") {
			t.Fatalf("gh-auth must never carry a token: %q", c.script)
		}
	}
}

func TestGhAuthLoginIsNeverRetriedButCheckIs(t *testing.T) {
	t.Run("check retries through transport failures", func(t *testing.T) {
		io := newFakeIO().
			on("gh auth status -h github.com", "", ErrTransport).
			on("gh auth status -h github.com", "", ErrTransport).
			on("gh auth status -h github.com", "", nil)
		sleeper := &sleepRecorder{}
		p := ghAuthPlan()
		p.Steps[0].Retry = updplan.Retry{
			Attempts: 3, On: []updplan.RetryOn{updplan.RetryOnTransport},
			Backoff: updplan.Backoff{Initial: time.Millisecond, Max: time.Millisecond, Factor: 1},
		}
		e := Executor{IO: io, Sleep: sleeper.Sleep, Rand: midpointRand}
		rep := e.RunHost("alpha", p)
		r, ok := resultFor(rep, "gh.auth")
		if !ok || r.Status != OK || r.Attempts != 3 {
			t.Fatalf("check must retry through transport failures: %+v", r)
		}
		if len(io.interactiveCalls()) != 0 {
			t.Fatalf("the check succeeding on retry must never reach login: %+v", io.interactiveCalls())
		}
	})

	t.Run("a failing login is never retried", func(t *testing.T) {
		io := newFakeIO().
			on("gh auth status -h github.com", "", realExitError(t, 1)).
			onInteractive("gh auth login -h github.com", errDeviceFlowDenied)
		e := Executor{IO: io}
		rep := e.RunHost("alpha", ghAuthPlan())
		r, ok := resultFor(rep, "gh.auth")
		if !ok || r.Status != Failed {
			t.Fatalf("a failing login must fail the step: %+v", r)
		}
		if len(io.interactiveCalls()) != 1 {
			t.Fatalf("login must be attempted exactly once, never retried: %d calls", len(io.interactiveCalls()))
		}
		if len(io.batchCalls()) != 1 {
			t.Fatalf("a failed login must not trigger a re-check: %d Batch calls", len(io.batchCalls()))
		}
	})
}

// --- task 15: retries with backoff under per-attempt timeouts ----------------

// retryPlan builds a one-step (batch, non-interactive run) plan with the
// given retry policy, so the retry/backoff/timeout machinery can be
// exercised directly.
func retryPlan(retry updplan.Retry, timeout time.Duration, expect updplan.Expect) updplan.Plan {
	st := runStep("x", "", "echo hi")
	st.Retry = retry
	st.Timeout = timeout
	if expect.Exit != nil {
		st.Expect = expect
	}
	return updplan.Plan{Steps: []updplan.Step{st}}
}

func stdBackoff() updplan.Backoff {
	return updplan.Backoff{Initial: 5 * time.Second, Max: 2 * time.Minute, Factor: 2, Jitter: true}
}

func TestTransportFailureIsRetriedWithBackoff(t *testing.T) {
	io := newFakeIO().
		on("echo hi", "", ErrTransport).
		on("echo hi", "", ErrTransport).
		on("echo hi", "", nil)
	sleeper := &sleepRecorder{}
	retry := updplan.Retry{Attempts: 3, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: stdBackoff()}
	e := Executor{IO: io, Sleep: sleeper.Sleep, Rand: midpointRand}
	rep := e.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r, _ := resultFor(rep, "x")
	if r.Status != OK || r.Attempts != 3 {
		t.Fatalf("r = %+v, want ok after 3 attempts", r)
	}
	waits := sleeper.all()
	if len(waits) != 2 || waits[0] != 5*time.Second || waits[1] != 10*time.Second {
		t.Fatalf("waits = %v, want [5s 10s]", waits)
	}
}

func TestNonMatchingFailureIsNotRetried(t *testing.T) {
	io := newFakeIO().on("echo hi", "", realExitError(t, 1))
	retry := updplan.Retry{Attempts: 3, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: stdBackoff()}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r, _ := resultFor(rep, "x")
	if r.Status != Failed || r.Attempts != 1 {
		t.Fatalf("a plain exit failure not in retry.on must not be retried: %+v", r)
	}
}

func TestRetryOnAnyRetriesEveryUnexpectedExit(t *testing.T) {
	io := newFakeIO().on("echo hi", "", realExitError(t, 1)).on("echo hi", "", nil)
	retry := updplan.Retry{Attempts: 2, On: []updplan.RetryOn{updplan.RetryOnAny}, Backoff: updplan.Backoff{Initial: time.Millisecond, Factor: 1}}
	e := Executor{IO: io, Sleep: func(time.Duration) {}}
	rep := e.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r, _ := resultFor(rep, "x")
	if r.Status != OK || r.Attempts != 2 {
		t.Fatalf("retry.on: any must retry a plain exit failure: %+v", r)
	}
}

func TestRetryOnExitCodeMatchesOnlyThatCode(t *testing.T) {
	io := newFakeIO().on("echo hi", "", realExitError(t, 7)).on("echo hi", "", nil)
	retry := updplan.Retry{Attempts: 2, On: []updplan.RetryOn{"exit:7"}, Backoff: updplan.Backoff{Initial: time.Millisecond, Factor: 1}}
	e := Executor{IO: io, Sleep: func(time.Duration) {}}
	rep := e.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r, _ := resultFor(rep, "x")
	if r.Status != OK || r.Attempts != 2 {
		t.Fatalf("retry.on: [exit:7] must retry an exit-7 failure: %+v", r)
	}

	io2 := newFakeIO().on("echo hi", "", realExitError(t, 9))
	e2 := Executor{IO: io2, Sleep: func(time.Duration) {}}
	rep2 := e2.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r2, _ := resultFor(rep2, "x")
	if r2.Status != Failed || r2.Attempts != 1 {
		t.Fatalf("retry.on: [exit:7] must NOT retry an exit-9 failure: %+v", r2)
	}
}

func TestExpectedExitIsNeverRetried(t *testing.T) {
	io := newFakeIO().on("echo hi", "", realExitError(t, 3))
	retry := updplan.Retry{Attempts: 3, On: []updplan.RetryOn{updplan.RetryOnAny}, Backoff: stdBackoff()}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", retryPlan(retry, 0, updplan.Expect{Exit: []int{3}}))
	r, _ := resultFor(rep, "x")
	if r.Status != OK || r.Attempts != 1 {
		t.Fatalf("an expected exit must never be retried: %+v", r)
	}
}

func TestAttemptsAreExhaustedThenOnFailureApplies(t *testing.T) {
	io := newFakeIO().on("echo hi", "", ErrTransport).on("echo hi", "", ErrTransport)
	retry := updplan.Retry{Attempts: 2, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: updplan.Backoff{Initial: time.Millisecond, Factor: 1}}
	st := runStep("x", "", "echo hi")
	st.Retry = retry
	after := runStep("after", "", "echo after", "x")
	p := updplan.Plan{Steps: []updplan.Step{st, after}}
	io.on("echo after", "", nil)
	e := Executor{IO: io, Sleep: func(time.Duration) {}}
	rep := e.RunHost("alpha", p)
	r, _ := resultFor(rep, "x")
	if r.Status != Failed || r.Attempts != 2 {
		t.Fatalf("attempts must be exhausted before failing: %+v", r)
	}
	dep, _ := resultFor(rep, "after")
	if dep.Status != DepFailed {
		t.Fatalf("on_failure: stop (default) must block the dependent: %+v", dep)
	}
}

func TestTimeoutCancelsTheAttempt(t *testing.T) {
	io := newFakeIO().blockOn("echo hi")
	retry := updplan.Retry{Attempts: 1, On: []updplan.RetryOn{}, Backoff: stdBackoff()}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", retryPlan(retry, 1*time.Second, stdExpect()))
	r, _ := resultFor(rep, "x")
	if !r.TimedOut {
		t.Fatalf("r.TimedOut = false, want true: %+v", r)
	}
	if r.Reason != "timed out after 1s" {
		t.Fatalf("Reason = %q, want %q", r.Reason, "timed out after 1s")
	}
}

func TestTimeoutIsRetriedOnlyWhenListed(t *testing.T) {
	t.Run("not listed -> single attempt", func(t *testing.T) {
		io := newFakeIO().blockOn("echo hi")
		retry := updplan.Retry{Attempts: 3, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: stdBackoff()}
		e := Executor{IO: io}
		rep := e.RunHost("alpha", retryPlan(retry, 10*time.Millisecond, stdExpect()))
		r, _ := resultFor(rep, "x")
		if r.Attempts != 1 {
			t.Fatalf("a timeout not in retry.on must not be retried: %+v", r)
		}
	})
	t.Run("listed -> retried", func(t *testing.T) {
		io := newFakeIO().blockOn("echo hi")
		retry := updplan.Retry{Attempts: 2, On: []updplan.RetryOn{updplan.RetryOnTimeout}, Backoff: updplan.Backoff{Initial: time.Millisecond, Factor: 1}}
		e := Executor{IO: io, Sleep: func(time.Duration) {}}
		rep := e.RunHost("alpha", retryPlan(retry, 10*time.Millisecond, stdExpect()))
		r, _ := resultFor(rep, "x")
		if r.Attempts != 2 {
			t.Fatalf("a timeout listed in retry.on must be retried: %+v", r)
		}
	})
}

func TestInteractiveStepsAreNeverRetried(t *testing.T) {
	io := newFakeIO().onInteractive("./install.sh", errDeviceFlowDenied)
	st := interactiveRunStep("x", "", "./install.sh")
	st.Retry = updplan.Retry{Attempts: 5, On: []updplan.RetryOn{updplan.RetryOnAny}, Backoff: stdBackoff()}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", updplan.Plan{Steps: []updplan.Step{st}})
	r, _ := resultFor(rep, "x")
	if r.Status != Failed || r.Attempts != 1 {
		t.Fatalf("an interactive step must never be retried even with retry.on: any: %+v", r)
	}
}

func TestInteractiveHasNoDeadlineUnlessSet(t *testing.T) {
	var sawDeadline bool
	io := recordingDeadlineIO{onCheck: func(ok bool) { sawDeadline = ok }}
	st := interactiveRunStep("x", "", "./install.sh")
	e := Executor{IO: io}
	e.RunHost("alpha", updplan.Plan{Steps: []updplan.Step{st}})
	if sawDeadline {
		t.Fatal("an interactive step with no explicit timeout must get no ctx deadline")
	}
}

// recordingDeadlineIO reports whether the ctx it was called with carries a
// deadline.
type recordingDeadlineIO struct{ onCheck func(bool) }

func (r recordingDeadlineIO) Batch(ctx context.Context, host string, st updplan.Step, script string) (string, error) {
	return "", nil
}
func (r recordingDeadlineIO) Interactive(ctx context.Context, host string, st updplan.Step, script string) error {
	_, ok := ctx.Deadline()
	r.onCheck(ok)
	return nil
}

func TestExecutorTimeoutOverridesBatchSteps(t *testing.T) {
	io := newFakeIO().blockOn("echo hi")
	st := runStep("x", "", "echo hi")
	st.Timeout = 10 * time.Minute // the plan's own timeout, should be overridden
	e := Executor{IO: io, Timeout: 20 * time.Millisecond}
	rep := e.RunHost("alpha", updplan.Plan{Steps: []updplan.Step{st}})
	r, _ := resultFor(rep, "x")
	if !r.TimedOut {
		t.Fatalf("Executor.Timeout must override the step's own batch timeout: %+v", r)
	}
}

func TestNoRetryForcesOneAttempt(t *testing.T) {
	io := newFakeIO().on("echo hi", "", ErrTransport)
	retry := updplan.Retry{Attempts: 5, On: []updplan.RetryOn{updplan.RetryOnTransport}, Backoff: stdBackoff()}
	e := Executor{IO: io, NoRetry: true}
	rep := e.RunHost("alpha", retryPlan(retry, 0, stdExpect()))
	r, _ := resultFor(rep, "x")
	if r.Attempts != 1 {
		t.Fatalf("--no-retry must force exactly one attempt: %+v", r)
	}
}
