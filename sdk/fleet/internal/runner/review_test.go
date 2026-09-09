package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// This file exists so review_test.go's peers in updexec can drive
// runner.Exec/runner.Fake through the RunInteractiveCtx capability added
// for the leaf B code-review finding on Console.Interactive
// (docs/mbo/plans/fleet-update.md). The behavioural tests that matter live
// in internal/updexec/review_test.go, which type-asserts for
// interactiveCtxRunner and exercises runner.Exec.RunInteractiveCtx directly
// against a stub `ssh` on PATH (TestInteractiveDeadlineKillsTheChild) —
// that is the only place a stub subprocess can prove the child is actually
// killed. The tests below cover Fake.RunInteractiveCtx directly, which
// updexec's fakeIO/recordingRunner doubles do not exercise.

// TestFakeRunInteractiveCtxHonoursCancellation proves Fake.RunInteractiveCtx
// mirrors Fake.RunStreamCtx's ctx handling: a Blocked host waits on
// ctx.Done() and returns ctx.Err(), so a test can drive the
// interactive-timeout path deterministically without a real process.
func TestFakeRunInteractiveCtxHonoursCancellation(t *testing.T) {
	f := Fake{Block: map[string]bool{"host": true}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- f.RunInteractiveCtx(ctx, "host", "cmd") }()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fake.RunInteractiveCtx did not honour the deadline within 2s")
	}
}

// TestFakeRunInteractiveCtxDelegatesWhenNotBlocked proves an unblocked host
// behaves exactly like RunInteractive: it returns immediately, using the
// same Err table.
func TestFakeRunInteractiveCtxDelegatesWhenNotBlocked(t *testing.T) {
	wantErr := errors.New("boom")
	f := Fake{Err: map[string]error{"host": wantErr}}
	if err := f.RunInteractiveCtx(context.Background(), "host", "cmd"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}

	f2 := Fake{}
	if err := f2.RunInteractiveCtx(context.Background(), "other", "cmd"); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

// TestExecImplementsInteractiveCtxRunner is a compile-time-adjacent check
// that runner.Exec satisfies the shape updexec.interactiveCtxRunner
// expects: RunInteractiveCtx(ctx, host string, argv ...string) error. A
// signature drift here would make Console.Interactive's type assertion
// silently stop matching and fall back to the unbounded path.
func TestExecImplementsInteractiveCtxRunner(t *testing.T) {
	var _ interface {
		RunInteractiveCtx(ctx context.Context, host string, argv ...string) error
	} = Exec{}
}
