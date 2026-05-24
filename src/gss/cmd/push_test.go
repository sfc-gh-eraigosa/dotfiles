package cmd

import (
	stderrors "errors"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
)

// TestClassicAllowed_ModeForceMatrix pins the mode gate (PR-23/26): a
// classic command is allowed in a regular checkout (regardless of
// --force-autonomous) and refused with ErrWrongMode inside a worker
// worktree (force does NOT bypass the gate). The worktree-detection logic
// itself now lives in internal/mode and is tested there.
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
