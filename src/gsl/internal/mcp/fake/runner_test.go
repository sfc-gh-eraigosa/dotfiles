package fake_test

import (
	"context"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/mcp"
	"github.com/wenlock/dotfiles/gsl/internal/mcp/fake"
)

// Compile-time assertion: *fake.Runner satisfies mcp.Runner.
var _ mcp.Runner = (*fake.Runner)(nil)

func TestFakeRunnerDefaultResponse(t *testing.T) {
	r := &fake.Runner{}
	out, err := r.Run(context.Background(), "mcp", "list")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for zero-value Response, got %v", out)
	}
}

func TestFakeRunnerScript(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"servers":["foo"]}`)},
		},
	}
	out, err := r.Run(context.Background(), "mcp", "list")
	if err != nil || string(out) != `{"servers":["foo"]}` {
		t.Errorf("got (%q, %v); want JSON output, nil", out, err)
	}
}

func TestFakeRunnerRecordsCalls(t *testing.T) {
	r := &fake.Runner{}
	r.Run(context.Background(), "mcp", "list")
	r.Run(context.Background(), "mcp", "status")
	if r.CallCount() != 2 {
		t.Errorf("CallCount = %d; want 2", r.CallCount())
	}
}

func TestFakeRunnerReset(t *testing.T) {
	r := &fake.Runner{}
	r.Run(context.Background(), "mcp", "list")
	r.Reset()
	if r.CallCount() != 0 {
		t.Errorf("after Reset, CallCount = %d; want 0", r.CallCount())
	}
}
