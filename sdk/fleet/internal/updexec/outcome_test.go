package updexec

import (
	"reflect"
	"testing"
)

// teeLikeRunHost wires c exactly as RunHostCtx does: every line ALSO lands
// in the capture, stderr with StderrMark.
func teeLikeRunHost(c Console, capture *[]string) StepIO {
	w := memWriter{capture}
	return c.withLines(
		func(_, line string) { w.Line(line) },
		func(_, line string) { w.Line(StderrMark + line) },
	)
}

const skipLine = "fleet: step dotfiles.sync skipped: " + reasonDirtySkip

// The TUI's case: a caller with ErrLine. The outcome must reach the caller's
// stderr (the live badge and errors pane) AND the capture with the mark, once
// each, so the dashboard and `fleet history` count the same warning.
func TestNoteOutcomeReachesTUIAndCaptureAlike(t *testing.T) {
	var tuiErr, tuiOut, capture []string
	io := teeLikeRunHost(Console{
		Line:    func(_, l string) { tuiOut = append(tuiOut, l) },
		ErrLine: func(_, l string) { tuiErr = append(tuiErr, l) },
	}, &capture)
	e := Executor{IO: io}
	e.noteOutcome(memWriter{&capture}, "h", Result{Step: "dotfiles.sync", Status: Skipped, Reason: reasonDirtySkip})

	if !reflect.DeepEqual(tuiErr, []string{skipLine}) {
		t.Fatalf("TUI stderr got %q, want exactly the outcome line", tuiErr)
	}
	if len(tuiOut) != 0 {
		t.Fatalf("outcome must not reach the TUI's stdout: %q", tuiOut)
	}
	if !reflect.DeepEqual(capture, []string{StderrMark + skipLine}) {
		t.Fatalf("capture got %q, want the line once, with StderrMark", capture)
	}
}

// The CLI's case: ErrLine nil. The terminal (Line) still shows the outcome,
// and the capture still marks it as stderr.
func TestNoteOutcomeReachesCLITerminalAndCapture(t *testing.T) {
	var term, capture []string
	io := teeLikeRunHost(Console{Line: func(_, l string) { term = append(term, l) }}, &capture)
	Executor{IO: io}.noteOutcome(memWriter{&capture}, "h", Result{Step: "dotfiles.sync", Status: Skipped, Reason: reasonDirtySkip})

	if !reflect.DeepEqual(term, []string{skipLine}) {
		t.Fatalf("terminal got %q", term)
	}
	if !reflect.DeepEqual(capture, []string{StderrMark + skipLine}) {
		t.Fatalf("capture got %q", capture)
	}
}

// A dependency-failed step is the cascade of a failure already counted: it is
// recorded as a plain (unmarked) capture line and never reaches stderr, so
// one dirty clone badges the run once, not once per blocked step. OK and
// Cancelled steps write nothing.
func TestNoteOutcomeDoesNotCountTheCascade(t *testing.T) {
	var tuiErr, capture []string
	io := teeLikeRunHost(Console{ErrLine: func(_, l string) { tuiErr = append(tuiErr, l) }}, &capture)
	e := Executor{IO: io}
	w := memWriter{&capture}

	e.noteOutcome(w, "h", Result{Step: "install", Status: DepFailed, Reason: "blocked by dotfiles.sync"})
	e.noteOutcome(w, "h", Result{Step: "a", Status: OK})
	e.noteOutcome(w, "h", Result{Step: "b", Status: Cancelled, Reason: "cancelled"})

	if len(tuiErr) != 0 {
		t.Fatalf("cascade / OK / cancelled must not reach stderr: %q", tuiErr)
	}
	want := []string{"fleet: step install dependency-failed: blocked by dotfiles.sync"}
	if !reflect.DeepEqual(capture, want) {
		t.Fatalf("capture got %q, want %q", capture, want)
	}
}
