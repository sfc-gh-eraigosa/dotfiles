package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
)

// Target is the repository (or organization) a verb acts on.
type Target struct {
	Owner string
	Repo  string // empty for an org-scoped run
}

func (t Target) String() string {
	if t.Repo == "" {
		return t.Owner
	}
	return t.Owner + "/" + t.Repo
}

// gitRemote is the test seam for reading the checkout's origin URL.
var gitRemote = func() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveTarget honours -R, else reads the checkout's origin remote.
func (g *Globals) resolveTarget() (Target, error) {
	if g.Target != "" {
		owner, repo, err := parseTarget(g.Target)
		if err != nil {
			return Target{}, err
		}
		return Target{Owner: owner, Repo: repo}, nil
	}
	remote, err := gitRemote()
	if err != nil {
		return Target{}, fmt.Errorf("%w: no -R given and this is not a git checkout with a github.com origin — pass -R owner/repo", ErrUsage)
	}
	owner, repo, err := gh.ParseRemote(remote)
	if err != nil {
		return Target{}, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return Target{Owner: owner, Repo: repo}, nil
}

// client resolves a credential and returns an authenticated client plus the
// source it came from.
func (g *Globals) client(ctx context.Context, t Target) (gh.Client, gh.Source, error) {
	c, src, err := gh.Resolve(ctx, gh.AuthOpts{Prefer: g.Auth, Owner: t.Owner, Repo: t.Repo})
	if err != nil {
		return nil, src, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return c, src, nil
}
