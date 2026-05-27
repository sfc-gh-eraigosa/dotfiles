package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/git"
	"github.com/wenlock/dotfiles/gsl/internal/git/fake"
)

// Compile-time assertion: *fake.Runner satisfies git.Runner.
var _ git.Runner = (*fake.Runner)(nil)

func TestFakeRunnerDefaultResponse(t *testing.T) {
	r := &fake.Runner{}
	out, err := r.Run(context.Background(), "status")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for zero-value Response, got %v", out)
	}
}

func TestFakeRunnerScriptFIFO(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("first"), Err: nil},
			{Stdout: []byte("second"), Err: nil},
		},
	}

	out, err := r.Run(context.Background(), "status")
	if err != nil || string(out) != "first" {
		t.Errorf("call 1: got (%q, %v); want ('first', nil)", out, err)
	}

	out, err = r.Run(context.Background(), "log")
	if err != nil || string(out) != "second" {
		t.Errorf("call 2: got (%q, %v); want ('second', nil)", out, err)
	}

	// Script exhausted; falls back to Default.
	out, err = r.Run(context.Background(), "diff")
	if err != nil || out != nil {
		t.Errorf("call 3 (default): got (%q, %v); want (nil, nil)", out, err)
	}
}

func TestFakeRunnerRecordsCalls(t *testing.T) {
	r := &fake.Runner{}
	r.Run(context.Background(), "status", "--porcelain")
	r.Run(context.Background(), "rev-parse", "--short", "HEAD")

	if r.CallCount() != 2 {
		t.Errorf("CallCount = %d; want 2", r.CallCount())
	}
	if r.Calls[0].Name != "status" {
		t.Errorf("Calls[0].Name = %q; want 'status'", r.Calls[0].Name)
	}
	if len(r.Calls[0].Args) != 1 || r.Calls[0].Args[0] != "--porcelain" {
		t.Errorf("Calls[0].Args = %v; want ['--porcelain']", r.Calls[0].Args)
	}
}

func TestFakeRunnerReset(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{{Stdout: []byte("data")}},
	}
	r.Run(context.Background(), "status")

	r.Reset()
	if r.CallCount() != 0 {
		t.Errorf("after Reset, CallCount = %d; want 0", r.CallCount())
	}
}

func TestFakeRunnerCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	r := &fake.Runner{
		Default: fake.Response{Stdout: []byte("should not reach")},
	}
	_, err := r.Run(ctx, "status")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	// Call was still recorded.
	if r.CallCount() != 1 {
		t.Errorf("call should be recorded even for cancelled ctx; CallCount = %d", r.CallCount())
	}
}

func TestFakeRunnerCombinesStdoutStderr(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("out"), Stderr: []byte("err")},
		},
	}
	out, _ := r.Run(context.Background(), "status")
	if string(out) != "outerr" {
		t.Errorf("combined output = %q; want 'outerr'", out)
	}
}
