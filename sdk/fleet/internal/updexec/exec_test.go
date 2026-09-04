package updexec

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

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
		for key, q := range f.batch {
			if strings.Contains(script, key) && len(q) > 0 {
				r = q[0]
				f.batch[key] = q[1:]
				break
			}
		}
	}
	f.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return r.out, r.err
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
