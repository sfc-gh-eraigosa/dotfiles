package identity

import (
	"regexp"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// branchRefRe matches a git branch/ref name: starts with a letter or digit
// (never "-"), then any run of letters, digits, ".", "_", "-", "/" (the
// last permits qualified names like "feature/foo").
//
// The leading-character rule is the security-relevant part (dotfiles#96,
// gosec finding): a value starting with "-" can be replayed as a git
// command-line flag rather than a ref name when it lands as a bare
// positional argument (e.g. a stored BaseBranch reaching
// `git rebase --onto <x> <base>`) — a value like "--exec=<cmd>" then runs
// <cmd> as a post-commit hook. Rejecting a leading "-" is necessary and
// sufficient to close that path; the rest of the pattern is an ordinary
// git ref-name shape, not a security boundary.
var branchRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// ValidateBranchRef validates a base/onto branch value before it is ever
// persisted to the registry (feature.Start's --base, worker.WorkerAdd's
// --base, feature.Restack's --onto) — the three write paths that feed a
// worker's BaseBranch, from where it is later replayed as an argument to
// `git rebase`.
func ValidateBranchRef(s string) error {
	if !branchRefRe.MatchString(s) {
		return errors.NewValidationError("base_branch", "must be a valid git ref name (a leading letter/digit, then letters, digits, '.', '_', '-', '/') and must not start with '-'")
	}
	return nil
}
