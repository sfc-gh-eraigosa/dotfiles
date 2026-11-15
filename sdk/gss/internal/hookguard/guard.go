package hookguard

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// Runner wraps a git.Runner and runs Check before every `git push` it is
// asked to make, so no push gss makes (classic push/pr, feature checkpoint,
// rebase, restack, merged …) can leave with the privacy guard switched off.
// Every other git call passes straight through. Production wires it around
// the system runner; there is deliberately no flag to turn it off.
type Runner struct {
	Inner git.Runner
}

// Run implements git.Runner.
func (g Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if dir, sub := invocation(name, args); sub == "push" {
		if err := Check(ctx, g.Inner, dir); err != nil {
			return nil, err
		}
	}
	return g.Inner.Run(ctx, name, args...)
}

// invocation finds the git subcommand and the directory it runs in from the
// Runner's (name, args) pair. gss composes calls as ("-C", dir, "push", …),
// so the first element may be a global option rather than the subcommand.
func invocation(name string, args []string) (dir, sub string) {
	toks := append([]string{name}, args...)
	dir = "."
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t == "-C" && i+1 < len(toks):
			i++
			if filepath.IsAbs(toks[i]) {
				dir = toks[i]
			} else {
				dir = filepath.Join(dir, toks[i])
			}
		case (t == "-c" || t == "--git-dir" || t == "--work-tree" || t == "--namespace") && i+1 < len(toks):
			i++
		case strings.HasPrefix(t, "-"):
			// --no-pager, --git-dir=…, -c… joined forms: no separate value.
		default:
			return dir, t
		}
	}
	return dir, ""
}

// verifier is the approval-token gate (satisfied by *approval.Verifier).
type verifier interface {
	Verify(ctx context.Context, repoPath string, forceAutonomous bool) error
}

// Approver runs Check before the approval-token gate. Every token-gated
// publish (classic push/pr, feature pr --ready) is refused up front, before
// the gate consumes the token, so the person fixing the override does not
// also have to re-issue the token. Runner remains the backstop for pushes
// that are not token-gated.
type Approver struct {
	Inner verifier
	Git   git.Runner
}

// Verify implements the approver interface used by classic and feature.
func (a Approver) Verify(ctx context.Context, repoPath string, forceAutonomous bool) error {
	if err := Check(ctx, a.Git, repoPath); err != nil {
		return err
	}
	return a.Inner.Verify(ctx, repoPath, forceAutonomous)
}
