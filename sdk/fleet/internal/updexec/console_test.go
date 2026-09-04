package updexec

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// recordingRunner is a minimal runner.Runner double that records every
// argv/stdin it was called with, for the Console/Background lane tests —
// these need to observe what actually reached the runner seam, which
// exec_test.go's fakeIO (a StepIO double) cannot see.
type recordingRunner struct {
	mu    sync.Mutex
	calls []struct {
		method string
		host   string
		stdin  string
		argv   []string
	}
	streamOut string
	streamErr error
}

func (r *recordingRunner) record(method, host, stdin string, argv []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, struct {
		method string
		host   string
		stdin  string
		argv   []string
	}{method, host, stdin, argv})
}

func (r *recordingRunner) Run(host string, argv ...string) (string, error) {
	r.record("Run", host, "", argv)
	return "", nil
}
func (r *recordingRunner) RunInteractive(host string, argv ...string) error {
	r.record("RunInteractive", host, "", argv)
	return nil
}
func (r *recordingRunner) RunStdin(host, stdin string, argv ...string) (string, error) {
	r.record("RunStdin", host, stdin, argv)
	return "", nil
}
func (r *recordingRunner) RunVia(peer, host string, argv ...string) (string, error) {
	r.record("RunVia", host, "", argv)
	return "", nil
}
func (r *recordingRunner) RunStream(host, stdin string, argv ...string) (<-chan string, <-chan error) {
	return r.RunStreamCtx(context.Background(), host, stdin, argv...)
}
func (r *recordingRunner) RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan string, <-chan error) {
	r.record("RunStreamCtx", host, stdin, argv)
	lines := make(chan string, 8)
	done := make(chan error, 1)
	for _, l := range strings.Split(r.streamOut, "\n") {
		if l != "" {
			lines <- l
		}
	}
	close(lines)
	done <- r.streamErr
	return lines, done
}

var _ runner.Runner = (*recordingRunner)(nil)

func (r *recordingRunner) last(method string) (string, string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.calls) - 1; i >= 0; i-- {
		if r.calls[i].method == method {
			return r.calls[i].host, r.calls[i].stdin, r.calls[i].argv
		}
	}
	return "", "", nil
}

func (r *recordingRunner) count(method string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c.method == method {
			n++
		}
	}
	return n
}

// --- task 16: Console and Background lanes ------------------------------

func TestConsoleStreamsBatchAndHandsOffInteractive(t *testing.T) {
	var lines []string
	r := &recordingRunner{streamOut: "line1\nline2"}
	c := Console{R: r, Line: func(host, l string) { lines = append(lines, l) }}

	out, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "x", Kind: updplan.KindSync}, "the-script")
	if err != nil {
		t.Fatal(err)
	}
	if out != "line1\nline2" {
		t.Fatalf("Batch output = %q", out)
	}
	if len(lines) != 2 || lines[0] != "line1" || lines[1] != "line2" {
		t.Fatalf("Line callback got %v, want [line1 line2]", lines)
	}

	if err := c.Interactive(context.Background(), "alpha", updplan.Step{ID: "y", Kind: updplan.KindRun}, "the-script"); err != nil {
		t.Fatal(err)
	}
	if r.count("RunInteractive") != 1 {
		t.Fatalf("Interactive must hand off to RunInteractive exactly once, got %d", r.count("RunInteractive"))
	}
}

func TestBackgroundRefusesInteractive(t *testing.T) {
	r := &recordingRunner{}
	b := Background{Console{R: r}}
	err := b.Interactive(context.Background(), "alpha", updplan.Step{ID: "x", Kind: updplan.KindRun}, "script")
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("err = %v, want ErrNoTerminal", err)
	}
	if r.count("RunInteractive") != 0 {
		t.Fatal("Background must never actually hand off the terminal")
	}
}

func TestPreambleAndStdinApplyToRunStepsOnly(t *testing.T) {
	r := &recordingRunner{}
	c := Console{
		R:        r,
		Preamble: func(st updplan.Step) string { return `sudo -S -p '' -v` },
		Stdin:    func(st updplan.Step) string { return "the-secret" },
	}

	if _, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "s", Kind: updplan.KindSync}, "sync-script"); err != nil {
		t.Fatal(err)
	}
	_, syncStdin, syncArgv := r.last("RunStreamCtx")
	if syncStdin != "" {
		t.Fatalf("a sync step must never receive stdin: %q", syncStdin)
	}
	if strings.Contains(strings.Join(syncArgv, " "), "sudo -S") {
		t.Fatalf("a sync step must never carry the preamble: %v", syncArgv)
	}

	if _, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "g", Kind: updplan.KindGhAuth}, "gh-script"); err != nil {
		t.Fatal(err)
	}
	_, ghStdin, ghArgv := r.last("RunStreamCtx")
	if ghStdin != "" || strings.Contains(strings.Join(ghArgv, " "), "sudo -S") {
		t.Fatalf("a gh-auth step must never receive stdin or the preamble: stdin=%q argv=%v", ghStdin, ghArgv)
	}

	if _, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "r", Kind: updplan.KindRun}, "run-script"); err != nil {
		t.Fatal(err)
	}
	_, runStdin, runArgv := r.last("RunStreamCtx")
	if runStdin != "the-secret" {
		t.Fatalf("a run step must receive Stdin's answer, got %q", runStdin)
	}
	if !strings.Contains(strings.Join(runArgv, " "), "sudo -S -p '' -v && run-script") {
		t.Fatalf("a run step's script must carry the preamble: %v", runArgv)
	}
}

func TestExitCodeMapsExitErrorAndSSH255(t *testing.T) {
	if exitCode(nil) != 0 {
		t.Fatal("exitCode(nil) must be 0")
	}
	if exitCode(ErrTransport) != 255 {
		t.Fatal("exitCode(ErrTransport) must be 255")
	}
	ee := exec.Command("sh", "-c", "exit 3").Run()
	if exitCode(ee) != 3 {
		t.Fatalf("exitCode(*exec.ExitError exit 3) = %d, want 3", exitCode(ee))
	}
	if exitCode(errors.New("some other error")) != -1 {
		t.Fatal("an unrecognised error must map to -1 (unknown)")
	}

	// Console.Batch must map an ssh exit of 255 to ErrTransport.
	code255 := exec.Command("sh", "-c", "exit "+strconv.Itoa(255)).Run()
	r := &recordingRunner{streamErr: code255}
	c := Console{R: r}
	_, err := c.Batch(context.Background(), "alpha", updplan.Step{ID: "x", Kind: updplan.KindSync}, "script")
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("Console.Batch on ssh exit 255 = %v, want ErrTransport", err)
	}
	if exitCode(err) != 255 {
		t.Fatalf("exitCode after wrapping must still recover 255, got %d", exitCode(err))
	}
}
