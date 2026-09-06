// Package fake provides a recording, scriptable implementation of
// git.Runner for use in tests.
//
// # Why this exists
//
// Per sdk/gss/docs/design.md ("Test seams"), every package that needs to
// call git must do so through git.Runner. Production wires the real
// implementation (git.SystemRunner); tests wire fake.Runner. The fake
// records every invocation into Calls so a test can assert "we ran
// exactly these git commands in this order", and it returns scripted
// responses from a FIFO Script so a test can shape the conversation
// (e.g. first call returns dirty porcelain output, second call returns
// clean).
//
// Script semantics (FIFO)
//
// The fake supports two response sources, checked in order:
//
//  1. Script ([]Response) — popped from the front on each Run call.
//  2. Default (Response) — returned when Script is empty.
//
// FIFO was chosen over map-keyed responses (e.g. by "<name> <args>") for
// three reasons: it keeps test setup compact (no key string ceremony),
// it forces tests to declare the expected order of git calls (improving
// signal on subtle ordering bugs), and it matches how the future
// orchestrators in internal/feature/* will be exercised — as a sequence
// of well-defined steps.
//
// # Concurrency
//
// All exported state (Calls, Script, Default) is guarded by a single
// mutex. The fake is safe to share across goroutines, which matters
// because future commands such as `gss feature checkpoint --auto` will
// fan out concurrent git invocations.
//
// # Default state
//
// A zero-value Runner returns Default == Response{} (exit 0, empty
// output, nil error) on every call. Tests that don't care about
// responses can use it as-is; tests that do, populate Script (and
// optionally override Default).
package fake

import (
	"context"
	"os"
	"sync"
)

// Response is the scripted return value from a single Run call. Either
// Err is non-nil (used verbatim and the bytes are still returned to the
// caller), or Err is nil and ExitCode is informational only (the fake
// does NOT synthesise an *exec.ExitError; tests that need a non-zero
// exit error should set Err to e.g. a *exec.ExitError stand-in).
//
// Stdout and Stderr are conceptually distinct but the real Runner
// returns combined output as a single []byte, so the fake concatenates
// Stdout then Stderr on the way out.
type Response struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

// CallRecord captures one Run invocation.
//
// Cwd is os.Getwd() at the time of the call, captured because the fake
// does not actually exec a subprocess and so has no cmd.Dir of its own.
// Tests that care about cwd behaviour should change directory before
// the call (or compose -C <path> into args, which is the canonical gss
// pattern per classic cmd/scan.go, cmd/push.go, cmd/status.go).
type CallRecord struct {
	Name string
	Args []string
	Cwd  string
}

// Runner implements git.Runner with a recording + scripting backend.
// Field assignment is fine; concurrent access goes through Run / Reset
// which take the mutex. Direct field reads from tests are safe between
// calls (the test owns the goroutine that called Run).
type Runner struct {
	// Default is returned when Script is exhausted.
	Default Response
	// Script is a FIFO of responses. Run pops from the front on each
	// call.
	Script []Response
	// Calls records every Run invocation in order.
	Calls []CallRecord

	mu sync.Mutex
}

// Run records the call and returns the next scripted response (or
// Default if the script is empty). The returned []byte is Stdout
// concatenated with Stderr to mirror the SystemRunner contract.
//
// ctx is currently honoured only at entry: if ctx is already cancelled
// when Run is called, the fake returns ctx.Err() ahead of any scripted
// response. This is a deliberate, narrow contract — the fake does not
// race the script against a select on ctx.Done() because there's no
// real subprocess to interrupt. Tests that need to exercise mid-run
// cancellation should use the real SystemRunner with a temp git repo.
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Honour an already-cancelled context up front so callers using
	// context.WithCancel/Deadline see the same cancellation semantics
	// they'd get from the real Runner.
	if err := ctx.Err(); err != nil {
		// Still record the call so tests can assert it was attempted.
		r.mu.Lock()
		r.Calls = append(r.Calls, recordOf(name, args))
		r.mu.Unlock()
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// argsCopy isolates the recorded slice from any caller-side mutation
	// of the variadic args after the call returns.
	argsCopy := append([]string(nil), args...)
	cwd, _ := os.Getwd()
	r.Calls = append(r.Calls, CallRecord{Name: name, Args: argsCopy, Cwd: cwd})

	resp := r.Default
	if len(r.Script) > 0 {
		resp = r.Script[0]
		r.Script = r.Script[1:]
	}

	out := combined(resp.Stdout, resp.Stderr)
	return out, resp.Err
}

// Reset clears Calls and Script and restores Default to its zero value.
// Use this between test cases when reusing a Runner.
func (r *Runner) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = nil
	r.Script = nil
	r.Default = Response{}
}

// CallCount returns the number of recorded invocations. Provided for
// convenience and to keep Calls slice reads concurrency-safe even when
// the test would otherwise read len(r.Calls) directly.
func (r *Runner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Calls)
}

// recordOf is a small helper so the cancelled-context path doesn't need
// to capture cwd (it would noise up the test signal and the call was
// never actually dispatched).
func recordOf(name string, args []string) CallRecord {
	argsCopy := append([]string(nil), args...)
	cwd, _ := os.Getwd()
	return CallRecord{Name: name, Args: argsCopy, Cwd: cwd}
}

// combined concatenates stdout and stderr in that order. Returning nil
// when both are empty keeps the fake's zero-Response path equivalent to
// the real Runner's "command produced no output" path.
func combined(stdout, stderr []byte) []byte {
	if len(stdout) == 0 && len(stderr) == 0 {
		return nil
	}
	out := make([]byte, 0, len(stdout)+len(stderr))
	out = append(out, stdout...)
	out = append(out, stderr...)
	return out
}
