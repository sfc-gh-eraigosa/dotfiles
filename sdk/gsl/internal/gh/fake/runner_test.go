package fake_test

import (
	"context"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
)

// Compile-time assertion: *fake.Runner satisfies gh.Runner.
var _ gh.Runner = (*fake.Runner)(nil)

func TestFakeRunnerDefaultResponse(t *testing.T) {
	r := &fake.Runner{}
	out, err := r.Run(context.Background(), "pr", "view")
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
			{Stdout: []byte(`{"number":42}`)},
		},
	}
	out, err := r.Run(context.Background(), "pr", "view", "--json", "number")
	if err != nil || string(out) != `{"number":42}` {
		t.Errorf("got (%q, %v); want JSON output, nil", out, err)
	}
}

func TestFakeRunnerRecordsCalls(t *testing.T) {
	r := &fake.Runner{}
	r.Run(context.Background(), "pr", "view")
	r.Run(context.Background(), "repo", "view")
	if r.CallCount() != 2 {
		t.Errorf("CallCount = %d; want 2", r.CallCount())
	}
}

func TestFakeRunnerReset(t *testing.T) {
	r := &fake.Runner{}
	r.Run(context.Background(), "pr", "view")
	r.Reset()
	if r.CallCount() != 0 {
		t.Errorf("after Reset, CallCount = %d; want 0", r.CallCount())
	}
}
