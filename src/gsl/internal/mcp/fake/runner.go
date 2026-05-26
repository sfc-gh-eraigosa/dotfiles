// Package fake provides a recording, scriptable implementation of
// mcp.Runner for use in tests.
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

// Runner implements mcp.Runner with a recording + scripting backend.
// A zero-value Runner returns Response{} on every call.
type Runner struct {
	Default Response
	Script  []Response
	Calls   []CallRecord

	mu sync.Mutex
}

// Run records the call and returns the next scripted response (or Default).
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		r.mu.Lock()
		argsCopy := append([]string(nil), args...)
		cwd, _ := os.Getwd()
		r.Calls = append(r.Calls, CallRecord{Name: name, Args: argsCopy, Cwd: cwd})
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

func combined(stdout, stderr []byte) []byte {
	if len(stdout) == 0 && len(stderr) == 0 {
		return nil
	}
	out := make([]byte, 0, len(stdout)+len(stderr))
	out = append(out, stdout...)
	out = append(out, stderr...)
	return out
}
