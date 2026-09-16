package identity_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

// TestValidateBranchRef_Valid pins the branch-name shapes that must keep
// working: bare names, qualified/hierarchical refs, dots and underscores.
func TestValidateBranchRef_Valid(t *testing.T) {
	for _, s := range []string{"main", "develop", "feature/foo", "release/1.2.3", "a_b-c"} {
		if err := identity.ValidateBranchRef(s); err != nil {
			t.Errorf("ValidateBranchRef(%q): %v; want nil", s, err)
		}
	}
}

// TestValidateBranchRef_RejectsFlagLike pins the dotfiles#96 fix: a value
// that would be read as a git option (starts with "-") must be rejected
// before it can ever reach the registry, since a value like
// "--exec=<cmd>" stored as a worker's BaseBranch and later replayed as a
// bare positional to `git rebase` runs <cmd> as a post-commit hook.
func TestValidateBranchRef_RejectsFlagLike(t *testing.T) {
	for _, s := range []string{"-", "--exec=touch /tmp/pwned", "--upload-pack=x", "-x"} {
		if err := identity.ValidateBranchRef(s); err == nil {
			t.Errorf("ValidateBranchRef(%q): err = nil; want a validation error", s)
		}
	}
}

func TestValidateBranchRef_RejectsEmpty(t *testing.T) {
	if err := identity.ValidateBranchRef(""); err == nil {
		t.Error("ValidateBranchRef(\"\"): err = nil; want a validation error")
	}
}
