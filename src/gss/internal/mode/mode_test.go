// Package mode_test verifies the canonical worker-mode detector per
// src/gss/docs/plan.md PR-26: IsInWorker(cwd, registry) returns
// (workerRef, true) inside a registered worktree, ("", false) otherwise.
package mode_test

import (
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/mode"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

func reg() registry.Registry {
	return registry.Registry{
		SchemaVersion: 1,
		Features: []registry.Feature{{
			Name: "feat",
			Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Suffix: "moss", Worktree: "/wt/erai/api-moss"},
				{User: "bot", Purpose: "ui", Worktree: "/wt/bot/ui"},
			},
		}},
	}
}

func TestIsInWorker(t *testing.T) {
	cases := []struct {
		cwd     string
		wantRef string
		wantIn  bool
	}{
		{"/wt/erai/api-moss", "feat/erai/api-moss", true},            // exact, with suffix
		{"/wt/erai/api-moss/internal/x", "feat/erai/api-moss", true}, // under
		{"/wt/bot/ui", "feat/bot/ui", true},                          // exact, no suffix
		{"/wt/erai/api", "", false},                                  // prefix but not a path boundary
		{"/elsewhere", "", false},                                    // unrelated
		{"", "", false},                                              // empty cwd
	}
	for _, c := range cases {
		ref, in := mode.IsInWorker(c.cwd, reg())
		if in != c.wantIn || ref != c.wantRef {
			t.Errorf("IsInWorker(%q) = (%q, %v); want (%q, %v)", c.cwd, ref, in, c.wantRef, c.wantIn)
		}
	}
}

func TestIsInWorker_EmptyRegistry(t *testing.T) {
	if ref, in := mode.IsInWorker("/anywhere", registry.Registry{}); in || ref != "" {
		t.Errorf("empty registry → (%q, %v); want (\"\", false)", ref, in)
	}
}
