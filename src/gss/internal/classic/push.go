// Package classic orchestrates the plain (non-feature) gss commands —
// `gss push` and `gss pr` — by composing the foundation packages. This
// file is the push flow (design.md → "Existing features that must survive
// the refactor"): approval → backup → sync → push → auto-PR, reproducing
// the behaviour of the classic cmd/push.go.
//
// The backup / sync / approval dependencies are narrow interfaces so the
// orchestration is unit-testable with trivial fakes; production wires the
// concrete services (internal/backup, internal/sync, internal/approval).
package classic

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/sync"
)

// approver gates the push on a HEAD-bound approval token.
type approver interface {
	Verify(ctx context.Context, repoPath string, forceAutonomous bool) error
}

// backuper creates the pre-push safety branch and returns its name.
type backuper interface {
	Create(ctx context.Context, repoPath string) (string, error)
}

// syncer fetches + rebases before the push.
type syncer interface {
	Sync(ctx context.Context, repoPath string) (sync.Result, error)
}

// Pusher runs the classic push flow.
type Pusher struct {
	Git      git.Runner
	GH       gh.Client
	Approval approver
	Backup   backuper
	Sync     syncer
	Out      io.Writer
}

// PushOpts configures one push.
type PushOpts struct {
	RepoPath        string
	DefaultBranch   string
	ForceAutonomous bool
	// PRTitle/PRBody seed an auto-created PR on a feature branch; PRTitle
	// defaults to the branch name when empty.
	PRTitle string
	PRBody  string
}

// Push runs: approval handshake → safety backup → fetch+rebase sync →
// push (with -u on a feature branch) → surface-or-create a PR when not on
// the default branch. The approval failure aborts before any side effect.
func (p *Pusher) Push(ctx context.Context, opts PushOpts) error {
	if err := p.Approval.Verify(ctx, opts.RepoPath, opts.ForceAutonomous); err != nil {
		return err
	}

	branch := p.currentBranch(ctx, opts.RepoPath)
	isDefault := branch == opts.DefaultBranch

	fmt.Fprintln(p.Out, "Step 1: Creating safety backup...")
	name, err := p.Backup.Create(ctx, opts.RepoPath)
	if err != nil {
		return fmt.Errorf("classic push: backup: %w", err)
	}
	fmt.Fprintf(p.Out, "Backup branch: %s\n", name)

	fmt.Fprintf(p.Out, "Step 2: Syncing %s with origin/%s...\n", opts.RepoPath, branch)
	if _, err := p.Sync.Sync(ctx, opts.RepoPath); err != nil {
		return fmt.Errorf("classic push: sync: %w", err)
	}

	fmt.Fprintf(p.Out, "Step 3: Pushing %s to origin/%s...\n", opts.RepoPath, branch)
	pushArgs := []string{"-C", opts.RepoPath, "push"}
	if !isDefault {
		pushArgs = append(pushArgs, "-u") // upstream tracking on feature branches
	}
	pushArgs = append(pushArgs, "origin", branch)
	if out, err := p.Git.Run(ctx, pushArgs[0], pushArgs[1:]...); err != nil {
		return fmt.Errorf("classic push: push: %w: %s", err, strings.TrimSpace(string(out)))
	}
	fmt.Fprintln(p.Out, "Successfully pushed changes!")

	if !isDefault {
		if err := p.ensurePR(ctx, branch, opts); err != nil {
			// Non-fatal: the push already succeeded.
			fmt.Fprintf(p.Out, "PR step: %v\n", err)
		}
	}
	return nil
}

// ensurePR surfaces an existing open PR for branch, or creates one.
func (p *Pusher) ensurePR(ctx context.Context, branch string, opts PushOpts) error {
	prs, err := p.GH.PRList(ctx, gh.PRFilter{State: "open", Head: branch})
	if err != nil {
		return err
	}
	if len(prs) > 0 {
		fmt.Fprintf(p.Out, "Pull Request: %s\n", prs[0].URL)
		return nil
	}
	title := opts.PRTitle
	if title == "" {
		title = branch
	}
	pr, err := p.GH.PRCreate(ctx, gh.PRCreateOpts{
		Title: title, Body: opts.PRBody, Base: opts.DefaultBranch, Head: branch,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(p.Out, "Pull Request created: %s\n", pr.URL)
	return nil
}

// currentBranch returns the abbreviated HEAD ref, defaulting to "main".
func (p *Pusher) currentBranch(ctx context.Context, repoPath string) string {
	out, err := p.Git.Run(ctx, "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main"
	}
	if b := strings.TrimSpace(string(out)); b != "" {
		return b
	}
	return "main"
}
