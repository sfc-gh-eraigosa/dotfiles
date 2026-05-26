// Package fake provides a recording, scriptable implementation of
// git.Runner for use in tests.
//
// A zero-value Runner returns Response{} (exit 0, empty output, nil error)
// on every call. Tests populate Script for sequential scripted responses,
// or set Default for a uniform response.
package fake

import (
	"context"
	"os"
	"sync"
)

// Response is the scripted return value from a single Run call.
type Response struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

// CallRecord captures one Run invocation.
type CallRecord struct {
	Name string
	Args []string
	Cwd  string
}

// Runner implements git.Runner with a recording + scripting backend.
type Runner struct {
	// Default is returned when Script is exhausted.
	Default Response
	// Script is a FIFO of responses. Run pops from the front on each call.
	Script []Response
	// Calls records every Run invocation in order.
	Calls []CallRecord

	mu sync.Mutex
}

// Run records the call and returns the next scripted response (or Default
// if the script is empty). The returned []byte is Stdout concatenated with
// Stderr to mirror the SystemRunner contract.
//
// If ctx is already cancelled when Run is called, ctx.Err() is returned
// immediately (the call is still recorded).
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		r.mu.Lock()
		r.Calls = append(r.Calls, recordOf(name, args))
		r.mu.Unlock()
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

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
func (r *Runner) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = nil
	r.Script = nil
	r.Default = Response{}
}

// CallCount returns the number of recorded invocations.
func (r *Runner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Calls)
}

func recordOf(name string, args []string) CallRecord {
	argsCopy := append([]string(nil), args...)
	cwd, _ := os.Getwd()
	return CallRecord{Name: name, Args: argsCopy, Cwd: cwd}
}

func combined(stdout, stderr []byte) []byte {
	if len(stdout) == 0 && len(stderr) == 0 {
		return nil
	}
	out := make([]byte, 0, len(stdout)+len(stderr))
	out = append(out, stdout...)
	out = append(out, stderr...)
	return out
}
