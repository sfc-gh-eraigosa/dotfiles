// Package fake_test verifies the recording / scriptable git.Runner fake
// per src/gss/docs/plan.md PR-02.
//
// The fake is a contract that downstream packages (internal/feature/*,
// internal/sync, internal/scan, …) depend on for hermetic offline tests.
// Its observable behaviour — Calls is append-only and in-order, Script
// is FIFO, Default is the fallback, Reset wipes state, and the whole
// thing is safe under concurrent Run calls — is therefore part of the
// public contract and is covered here.
package fake_test

import (
	"context"
	"io"
	"reflect"
	"sync"
	"testing"

	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
)

// TestFake_RecordsCalls — every Run invocation must be captured in
// Calls in order. The args slice must be a defensive copy so that
// post-call mutation by the caller cannot retro-corrupt the record.
func TestFake_RecordsCalls(t *testing.T) {
	var r gitfake.Runner
	ctx := context.Background()

	if _, err := r.Run(ctx, "status", "--porcelain"); err != nil {
		t.Fatalf("Run 1: unexpected err: %v", err)
	}
	if _, err := r.Run(ctx, "rev-parse", "HEAD"); err != nil {
		t.Fatalf("Run 2: unexpected err: %v", err)
	}
	if _, err := r.Run(ctx, "diff", "--stat", "HEAD~1", "HEAD"); err != nil {
		t.Fatalf("Run 3: unexpected err: %v", err)
	}

	if got, want := r.CallCount(), 3; got != want {
		t.Fatalf("CallCount = %d; want %d", got, want)
	}
	if r.Calls[0].Name != "status" || !reflect.DeepEqual(r.Calls[0].Args, []string{"--porcelain"}) {
		t.Errorf("Calls[0] = %+v; want {status [--porcelain]}", r.Calls[0])
	}
	if r.Calls[1].Name != "rev-parse" || !reflect.DeepEqual(r.Calls[1].Args, []string{"HEAD"}) {
		t.Errorf("Calls[1] = %+v; want {rev-parse [HEAD]}", r.Calls[1])
	}
	if r.Calls[2].Name != "diff" || !reflect.DeepEqual(r.Calls[2].Args, []string{"--stat", "HEAD~1", "HEAD"}) {
		t.Errorf("Calls[2] = %+v; want {diff [--stat HEAD~1 HEAD]}", r.Calls[2])
	}
}

// TestFake_ArgsCopyDefensive — the recorded args slice must not alias
// caller storage. Mutating the caller's slice after Run must not change
// what was recorded.
func TestFake_ArgsCopyDefensive(t *testing.T) {
	var r gitfake.Runner
	args := []string{"--porcelain"}
	if _, err := r.Run(context.Background(), "status", args...); err != nil {
		t.Fatalf("Run: %v", err)
	}
	args[0] = "--MUTATED"
	if r.Calls[0].Args[0] != "--porcelain" {
		t.Errorf("recorded arg corrupted by caller mutation: got %q want %q",
			r.Calls[0].Args[0], "--porcelain")
	}
}

// TestFake_ConsumesScript — Script is popped FIFO; each Run gets the
// next response in order.
func TestFake_ConsumesScript(t *testing.T) {
	r := gitfake.Runner{
		Script: []gitfake.Response{
			{Stdout: []byte("first\n")},
			{Stdout: []byte("second\n")},
		},
	}
	ctx := context.Background()

	got1, err := r.Run(ctx, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if string(got1) != "first\n" {
		t.Errorf("Run 1 output = %q; want %q", got1, "first\n")
	}

	got2, err := r.Run(ctx, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if string(got2) != "second\n" {
		t.Errorf("Run 2 output = %q; want %q", got2, "second\n")
	}

	if len(r.Script) != 0 {
		t.Errorf("Script should be drained; got %d remaining", len(r.Script))
	}
}

// TestFake_CombinesStdoutAndStderr — the SystemRunner returns combined
// output, so the fake concatenates Stdout then Stderr.
func TestFake_CombinesStdoutAndStderr(t *testing.T) {
	r := gitfake.Runner{
		Script: []gitfake.Response{
			{Stdout: []byte("OUT\n"), Stderr: []byte("ERR\n")},
		},
	}
	got, err := r.Run(context.Background(), "status")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(got) != "OUT\nERR\n" {
		t.Errorf("combined = %q; want %q", got, "OUT\nERR\n")
	}
}

// TestFake_FallsBackToDefault — once Script is empty, Default is
// returned for every subsequent call.
func TestFake_FallsBackToDefault(t *testing.T) {
	r := gitfake.Runner{
		Default: gitfake.Response{Stdout: []byte("DEFAULT\n")},
		Script:  []gitfake.Response{{Stdout: []byte("scripted\n")}},
	}
	ctx := context.Background()

	got1, _ := r.Run(ctx, "x")
	got2, _ := r.Run(ctx, "x")
	got3, _ := r.Run(ctx, "x")

	if string(got1) != "scripted\n" {
		t.Errorf("first call: got %q want %q", got1, "scripted\n")
	}
	if string(got2) != "DEFAULT\n" || string(got3) != "DEFAULT\n" {
		t.Errorf("default fallback: got %q,%q want DEFAULT,DEFAULT", got2, got3)
	}
}

// TestFake_FallsBackToZeroDefault — an unconfigured Runner returns the
// zero Response (nil bytes, nil err) on every call.
func TestFake_FallsBackToZeroDefault(t *testing.T) {
	var r gitfake.Runner
	got, err := r.Run(context.Background(), "anything")
	if err != nil {
		t.Errorf("zero-value Run: err = %v; want nil", err)
	}
	if got != nil {
		t.Errorf("zero-value Run: bytes = %q; want nil", got)
	}
}

// TestFake_ErrorFromResponse — when a scripted Response.Err is set, Run
// returns that error verbatim. Bytes are still returned (the SystemRunner
// returns its accumulated buffer on exit error, so the fake mirrors that).
func TestFake_ErrorFromResponse(t *testing.T) {
	wantErr := io.EOF
	r := gitfake.Runner{
		Script: []gitfake.Response{
			{Stdout: []byte("partial\n"), Err: wantErr},
		},
	}
	got, err := r.Run(context.Background(), "status")
	if err != wantErr {
		t.Errorf("err = %v; want %v", err, wantErr)
	}
	if string(got) != "partial\n" {
		t.Errorf("bytes on error path = %q; want %q", got, "partial\n")
	}
}

// TestFake_CancelledContext — a context already cancelled at Run time
// returns ctx.Err() and records the attempted call (so tests can assert
// "we tried to run X but were cancelled").
func TestFake_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var r gitfake.Runner
	_, err := r.Run(ctx, "status", "--porcelain")
	if err != context.Canceled {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if r.CallCount() != 1 {
		t.Errorf("CallCount = %d; want 1 (call should be recorded even on cancel)", r.CallCount())
	}
}

// TestFake_ConcurrentSafe — 100 goroutines calling Run in parallel must
// produce exactly 100 recorded calls and no data race (verified by
// `go test -race`).
func TestFake_ConcurrentSafe(t *testing.T) {
	var r gitfake.Runner
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = r.Run(context.Background(), "status", "--porcelain")
		}()
	}
	wg.Wait()

	if got := r.CallCount(); got != n {
		t.Errorf("CallCount = %d; want %d", got, n)
	}
}

// TestFake_Reset — Reset wipes Calls, Script, and Default. A reused
// Runner behaves as if newly constructed.
func TestFake_Reset(t *testing.T) {
	r := gitfake.Runner{
		Default: gitfake.Response{Stdout: []byte("D\n")},
		Script:  []gitfake.Response{{Stdout: []byte("S\n")}},
	}
	if _, err := r.Run(context.Background(), "status"); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	r.Reset()

	if r.CallCount() != 0 {
		t.Errorf("after Reset, CallCount = %d; want 0", r.CallCount())
	}
	if len(r.Script) != 0 {
		t.Errorf("after Reset, len(Script) = %d; want 0", len(r.Script))
	}
	got, err := r.Run(context.Background(), "status")
	if err != nil {
		t.Errorf("post-Reset Run: err = %v; want nil", err)
	}
	if got != nil {
		t.Errorf("post-Reset Run: bytes = %q; want nil (zero Default)", got)
	}
}

// TestFake_RecordsCwd — CallRecord.Cwd captures the process cwd at the
// time of the call (used to validate code that changes dirs before
// invoking git). os.Getwd may fail (very rare), so we only assert
// non-emptiness here.
func TestFake_RecordsCwd(t *testing.T) {
	var r gitfake.Runner
	if _, err := r.Run(context.Background(), "status"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Calls[0].Cwd == "" {
		t.Errorf("Cwd not captured (got empty); expected the current working directory")
	}
}
