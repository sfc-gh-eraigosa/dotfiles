package feature

import "context"

// gitRebase runs `git rebase` with the positional upstream ref ALWAYS
// preceded by a "--" end-of-options separator (dotfiles#96, PR #333
// review) — the one invariant every git-rebase call site in this package
// must hold, centralized here so a future call site built by copying an
// existing one can't add itself without it. ontoFlag is the "--onto
// <target>" args to insert before "--", or nil for a plain rebase onto
// upstream.
func (s *Service) gitRebase(ctx context.Context, worktree string, ontoFlag []string, upstream string) ([]byte, error) {
	args := append([]string{worktree, "rebase"}, ontoFlag...)
	args = append(args, "--", upstream)
	return s.Git.Run(ctx, "-C", args...)
}
