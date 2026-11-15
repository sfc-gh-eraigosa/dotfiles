package hookguard

import (
	"context"
	"fmt"
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
	if inv := invocation(name, args); inv.sub == "push" {
		if inv.override != "" {
			return nil, fmt.Errorf("%w\ngss refuses to push %s: the command line sets %s, so the privacy guard (pre-push) would not run", ErrOverride, inv.dir, inv.override)
		}
		if err := Check(ctx, g.Inner, inv.dir); err != nil {
			return nil, err
		}
	}
	return g.Inner.Run(ctx, name, args...)
}

// call is what invocation reads from a Runner call: the git subcommand, the
// directory it runs in, and a core.hooksPath set on the command line.
type call struct {
	dir, sub, override string
}

// invocation finds the git subcommand and the directory it runs in from the
// Runner's (name, args) pair. gss composes calls as ("-C", dir, "push", …),
// so the first element may be a global option rather than the subcommand.
// --work-tree (or, without it, --git-dir) names the repo; a -c or
// --config-env that sets core.hooksPath is reported as an override.
func invocation(name string, args []string) call {
	toks := append([]string{name}, args...)
	c := call{dir: "."}
	var gitDir, workTree string
	at := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(c.dir, p)
	}
	value := func(i *int, t, flag string) (string, bool) {
		if t == flag && *i+1 < len(toks) {
			*i++
			return toks[*i], true
		}
		if v, ok := strings.CutPrefix(t, flag+"="); ok {
			return v, true
		}
		return "", false
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if v, ok := value(&i, t, "-C"); ok {
			c.dir = at(v)
			continue
		}
		if v, ok := value(&i, t, "--git-dir"); ok {
			gitDir = at(v)
			continue
		}
		if v, ok := value(&i, t, "--work-tree"); ok {
			workTree = at(v)
			continue
		}
		if _, ok := value(&i, t, "--namespace"); ok {
			continue
		}
		if t == "-c" && i+1 < len(toks) {
			i++
			c.hooksPathIn(toks[i])
			continue
		}
		if v, ok := strings.CutPrefix(t, "-c"); ok && !strings.HasPrefix(t, "--") {
			c.hooksPathIn(v)
			continue
		}
		if v, ok := strings.CutPrefix(t, "--config-env="); ok {
			c.hooksPathIn(v)
			continue
		}
		if strings.HasPrefix(t, "-") {
			continue // --no-pager, --bare, --exec-path=… and the like: no separate value
		}
		c.sub = t
		break
	}
	switch {
	case workTree != "":
		c.dir = workTree
	case gitDir != "":
		c.dir = gitDir
	}
	return c
}

// hooksPathIn records kv when it sets core.hooksPath (git config keys are
// case-insensitive in their section and name).
func (c *call) hooksPathIn(kv string) {
	if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, Key) {
		c.override = kv
	}
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
