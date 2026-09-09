package updexec

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func TestErrLineReceivesStderrOnly(t *testing.T) {
	f := runner.Fake{
		Out:    map[string]string{"h": "progress"},
		ErrOut: map[string]string{"h": "WARNING: apt-get update failed"},
	}
	var out, errs []string
	c := Console{
		R:       f,
		Line:    func(_, l string) { out = append(out, l) },
		ErrLine: func(_, l string) { errs = append(errs, l) },
	}
	got, err := c.Batch(context.Background(), "h", updplan.Step{Kind: updplan.KindRun}, "script")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] != "progress" {
		t.Fatalf("Line got %q", out)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "apt-get") {
		t.Fatalf("ErrLine got %q", errs)
	}
	// The returned output keeps BOTH streams: it is the step's captured text
	// and the source of a row's FAIL explanation.
	if !strings.Contains(got, "progress") || !strings.Contains(got, "apt-get") {
		t.Fatalf("Batch output must keep both streams, got %q", got)
	}
}

// Nil ErrLine is the CLI's case: stderr must still reach Line, i.e. today's
// behaviour byte for byte.
func TestNilErrLineRoutesStderrToLine(t *testing.T) {
	f := runner.Fake{Out: map[string]string{"h": "a"}, ErrOut: map[string]string{"h": "b"}}
	var seen []string
	c := Console{R: f, Line: func(_, l string) { seen = append(seen, l) }}
	if _, err := c.Batch(context.Background(), "h", updplan.Step{}, "s"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("with ErrLine nil, stderr must reach Line: %q", seen)
	}
}

// The package's EXISTING recordingRunner (console_test.go) implements
// runner.Runner and NOT runner.SplitStreamer — it is the real-world shape of
// this fallback, since every pre-existing test double in the repo is one.
func TestBatchFallsBackWhenNotSplitCapable(t *testing.T) {
	if _, ok := any(&recordingRunner{}).(runner.SplitStreamer); ok {
		t.Fatal("this test needs a runner WITHOUT the split capability")
	}
	r := &recordingRunner{streamOut: "x\ny"}
	var out, errs []string
	c := Console{
		R:       r,
		Line:    func(_, l string) { out = append(out, l) },
		ErrLine: func(_, l string) { errs = append(errs, l) },
	}
	if _, err := c.Batch(context.Background(), "h", updplan.Step{}, "s"); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(errs) != 0 {
		t.Fatalf("without the capability every line is stdout: out=%q err=%q", out, errs)
	}
}

// memOutput is a test updexec.Output that keeps every captured line.
//
// NAMES MATTER HERE: exec_test.go already declares `recordingOutput` and
// `recordingWriter` in this same package, so reusing either name fails to
// compile. Check for a collision before adding any package-level test type.
type memOutput struct{ lines *[]string }

func (m memOutput) Open(string, string) (LineWriter, string) {
	return memWriter(m), "mem://capture"
}

type memWriter struct{ lines *[]string }

func (w memWriter) Line(s string) { *w.lines = append(*w.lines, s) }
func (w memWriter) Close(string)  {}

func TestCaptureMarksStderr(t *testing.T) {
	var captured []string
	f := runner.Fake{
		Out:    map[string]string{"h": "installing"},
		ErrOut: map[string]string{"h": "WARNING: apt-get update failed"},
	}
	ex := Executor{IO: Console{R: f}, Out: memOutput{&captured}}
	ex.RunHost("h", updplan.Default())

	var sawOut, sawErr bool
	for _, l := range captured {
		if l == "installing" {
			sawOut = true
		}
		if l == StderrMark+"WARNING: apt-get update failed" {
			sawErr = true
		}
	}
	if !sawOut {
		t.Fatalf("stdout must reach the capture unprefixed: %q", captured)
	}
	if !sawErr {
		t.Fatalf("stderr must reach the capture marked %q: %q", StderrMark, captured)
	}
}

// TestInteractiveStepSaysItsOutputWentToTheTerminal pins that a capture is
// HONEST about what it does not contain. An interactive step hands the
// terminal to the remote command (ssh -t), so fleet never sees a byte of it:
// the default plan's `./install.sh` can run for two minutes, do the entire
// job, and leave the capture holding its step banner and nothing else.
//
// Read back by `fleet history` that is indistinguishable from an install
// that produced no output at all — a real run against a host whose sudo was
// broken looked identical to the successful re-run that fixed it. Naming the
// gap costs one line and turns "apparently did nothing" into "went
// somewhere else".
func TestInteractiveStepSaysItsOutputWentToTheTerminal(t *testing.T) {
	var captured []string
	// A precheck the sync accepts, so the run reaches the interactive step.
	f := runner.Fake{Out: map[string]string{"h": "state=clean branch=main"}}
	ex := Executor{IO: Console{R: f}, Out: memOutput{&captured}}
	ex.RunHost("h", updplan.Default())

	var said bool
	for _, l := range captured {
		if strings.Contains(l, "interactive") && strings.Contains(l, "not captured") {
			said = true
		}
	}
	if !said {
		t.Fatalf("an interactive step must say its output was not captured, got:\n%q", captured)
	}
}

// TestBackgroundLaneDoesNotClaimAnInteractiveGap is the guard on the note
// above. "interactive" is a property of the LANE, not of the plan flag:
// Background runs an `interactive: true` run step as Batch and tees every
// line into the capture, so the note would be a lie there — and a costly
// one, since histindex reads it (and the empty-step shape it describes) as
// "this run was never observed" and refuses to call the host clean. Writing
// it on the TUI's lane inverted the exact signal it was added to provide.
func TestBackgroundLaneDoesNotClaimAnInteractiveGap(t *testing.T) {
	var captured []string
	f := runner.Fake{Out: map[string]string{"h": "state=clean branch=main"}}
	ex := Executor{IO: Background{Console{R: f}}, Out: memOutput{&captured}}
	ex.RunHost("h", updplan.Default())

	for _, l := range captured {
		if strings.Contains(l, "not captured") {
			t.Fatalf("the background lane captures a run step in full; it must not claim a gap: %q\n%q", l, captured)
		}
	}
}
