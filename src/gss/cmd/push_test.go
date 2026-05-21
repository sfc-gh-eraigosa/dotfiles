package cmd

import (
	stderrors "errors"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// TestClassicAllowed_ModeForceMatrix pins PR-23's matrix: a classic command
// is allowed in a regular checkout (regardless of --force-autonomous) and
// refused with ErrWrongMode inside a worker worktree (force does NOT bypass
// the mode gate).
func TestClassicAllowed_ModeForceMatrix(t *testing.T) {
	cases := []struct {
		inWorker bool
		force    bool
		wantErr  bool
	}{
		{false, false, false}, // regular checkout, no force → allowed
		{false, true, false},  // regular checkout, force    → allowed
		{true, false, true},   // worker worktree, no force  → ErrWrongMode
		{true, true, true},    // worker worktree, force     → ErrWrongMode
	}
	for _, c := range cases {
		err := classicAllowed(c.inWorker, c.force)
		if c.wantErr {
			if !stderrors.Is(err, errors.ErrWrongMode) {
				t.Errorf("classicAllowed(inWorker=%v, force=%v) = %v; want ErrWrongMode", c.inWorker, c.force, err)
			}
		} else if err != nil {
			t.Errorf("classicAllowed(inWorker=%v, force=%v) = %v; want nil", c.inWorker, c.force, err)
		}
	}
}

func TestIsWorkerWorktree(t *testing.T) {
	reg := registry.Registry{
		SchemaVersion: 1,
		Features: []registry.Feature{{
			Name: "feat",
			Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Suffix: "moss", Worktree: "/wt/erai/api-moss"},
			},
		}},
	}
	cases := []struct {
		cwd     string
		wantRef string
		wantIn  bool
	}{
		{"/wt/erai/api-moss", "feat/erai/api-moss", true},         // exact
		{"/wt/erai/api-moss/sub/dir", "feat/erai/api-moss", true}, // under
		{"/wt/erai/api", "", false},                               // prefix-but-not-path-boundary
		{"/somewhere/else", "", false},                            // unrelated
		{"", "", false},                                           // empty cwd
	}
	for _, c := range cases {
		ref, in := isWorkerWorktree(c.cwd, reg)
		if in != c.wantIn || ref != c.wantRef {
			t.Errorf("isWorkerWorktree(%q) = (%q, %v); want (%q, %v)", c.cwd, ref, in, c.wantRef, c.wantIn)
		}
	}
}

func TestIsWorkerWorktree_EmptyRegistry(t *testing.T) {
	if ref, in := isWorkerWorktree("/anywhere", registry.Registry{}); in || ref != "" {
		t.Errorf("empty registry → (%q, %v); want (\"\", false)", ref, in)
	}
}
