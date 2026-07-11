package classic

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// PRer runs the classic `gss pr` flow (design.md → "Existing features that
// must survive the refactor"): on the default branch it first cuts a
// timestamped feature branch, then pushes and surfaces-or-opens a PR.
//
// Unlike the classic cmd/pr.go (which had no handshake), the orchestrator
// consumes the approval token first — opening a PR is an outward action and
// gets the same safeguard as push (the PR-24 acceptance pins
// "approval-token consumed"). --force-autonomous bypasses the prompt.
type PRer struct {
	Git      git.Runner
	GH       gh.Client
	Approval approver
	Clock    config.Clock
	Out      io.Writer
}

// PROpts configures one pr run.
type PROpts struct {
	RepoPath        string
	DefaultBranch   string
	ForceAutonomous bool
	Title           string // PRCreate title; defaults to the branch name
	Body            string
}

// PR runs: approval handshake → (on default branch) create feature/gss-<ts>
// → push -u → surface an existing open PR or create one.
func (p *PRer) PR(ctx context.Context, opts PROpts) error {
	if err := p.Approval.Verify(ctx, opts.RepoPath, opts.ForceAutonomous); err != nil {
		return err
	}

	branch := p.currentBranch(ctx, opts.RepoPath)
	if branch == opts.DefaultBranch {
		branch = "feature/gss-" + p.Clock.Now().Format("20060102-150405")
		fmt.Fprintf(p.Out, "On default branch; creating feature branch in %s: %s\n", opts.RepoPath, branch)
		if out, err := p.Git.Run(ctx, "-C", opts.RepoPath, "checkout", "-b", branch); err != nil {
			return fmt.Errorf("classic pr: create branch: %w: %s", err, strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Fprintf(p.Out, "Using current branch: %s\n", branch)
	}

	fmt.Fprintf(p.Out, "Pushing %s to origin...\n", branch)
	if out, err := p.Git.Run(ctx, "-C", opts.RepoPath, "push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("classic pr: push: %w: %s", err, strings.TrimSpace(string(out)))
	}

	prs, err := p.GH.PRList(ctx, gh.PRFilter{State: "open", Head: branch})
	if err != nil {
		return fmt.Errorf("classic pr: list: %w", err)
	}
	if len(prs) > 0 {
		fmt.Fprintf(p.Out, "Pull Request already exists: %s\n", prs[0].URL)
		return nil
	}

	title := opts.Title
	if title == "" {
		title = branch
	}
	pr, err := p.GH.PRCreate(ctx, gh.PRCreateOpts{Title: title, Body: opts.Body, Base: opts.DefaultBranch, Head: branch})
	if err != nil {
		return fmt.Errorf("classic pr: create: %w", err)
	}
	fmt.Fprintf(p.Out, "Pull Request created: %s\n", pr.URL)
	return nil
}

func (p *PRer) currentBranch(ctx context.Context, repoPath string) string {
	out, err := p.Git.Run(ctx, "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main"
	}
	if b := strings.TrimSpace(string(out)); b != "" {
		return b
	}
	return "main"
}
