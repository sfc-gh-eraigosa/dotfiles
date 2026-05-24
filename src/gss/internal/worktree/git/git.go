// Package git is the v1 worktree.Backend: a thin wrapper around
// `git worktree add/remove/list` and `git status --porcelain=v2 -b`
// (design.md → "v1 implementation: git backend"). It is the only backend
// that ships in v1 and takes no new dependencies.
package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gitrun "github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/worktree"
)

// Name is the backend ID persisted in registry.json.
const Name = "git"

// Backend materializes worktrees with the git CLI via a git.Runner.
type Backend struct {
	repo   string // main repo path; "" → resolve from CWD per call
	runner gitrun.Runner
}

// New returns a git backend that resolves the repo from the current
// directory. Used by the worktree registry.
func New() worktree.Backend { return &Backend{runner: gitrun.NewSystemRunner()} }

// NewBackend returns a git backend bound to an explicit repo path and
// runner (used by callers that already know the repo, and by tests).
func NewBackend(repo string, runner gitrun.Runner) *Backend {
	return &Backend{repo: repo, runner: runner}
}

func init() { worktree.Register(Name, New) }

var _ worktree.Backend = (*Backend)(nil)

// Name returns the backend ID.
func (b *Backend) Name() string { return Name }

func (b *Backend) repoDir(ctx context.Context) (string, error) {
	if b.repo != "" {
		return b.repo, nil
	}
	out, err := b.runner.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("git backend: resolve repo: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Create materializes a worktree. If the branch already exists (e.g. a
// retry after Remove, which leaves the branch behind), the existing branch
// is checked out; otherwise a new branch is created from BaseCommit (or the
// tip of BaseBranch). rebase.updateRefs=true is set on the new worktree so
// `git rebase --update-refs` restacks descendants automatically.
func (b *Backend) Create(req worktree.CreateReq) (worktree.Info, error) {
	ctx := context.Background()
	repo, err := b.repoDir(ctx)
	if err != nil {
		return worktree.Info{}, err
	}

	var args []string
	if b.branchExists(ctx, repo, req.Branch) {
		args = []string{"-C", repo, "worktree", "add", req.Path, req.Branch}
	} else {
		start := req.BaseCommit
		if start == "" {
			start = req.BaseBranch
		}
		args = []string{"-C", repo, "worktree", "add", "-b", req.Branch, req.Path, start}
	}
	if out, err := b.runner.Run(ctx, args[0], args[1:]...); err != nil {
		return worktree.Info{}, fmt.Errorf("git backend: worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}

	if out, err := b.runner.Run(ctx, "-C", req.Path, "config", "rebase.updateRefs", "true"); err != nil {
		return worktree.Info{}, fmt.Errorf("git backend: set rebase.updateRefs: %w: %s", err, strings.TrimSpace(string(out)))
	}

	head := ""
	if out, err := b.runner.Run(ctx, "-C", req.Path, "rev-parse", "HEAD"); err == nil {
		head = strings.TrimSpace(string(out))
	}
	return worktree.Info{
		Path: req.Path, Branch: req.Branch, BaseBranch: req.BaseBranch,
		Backend: Name, HeadSHA: head,
	}, nil
}

// Remove tears down the worktree. `git worktree remove` already refuses on
// a dirty/untracked worktree unless --force, satisfying the contract.
func (b *Backend) Remove(path string, force bool) error {
	ctx := context.Background()
	repo, err := b.repoDir(ctx)
	if err != nil {
		return err
	}
	args := []string{"-C", repo, "worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	if out, err := b.runner.Run(ctx, args[0], args[1:]...); err != nil {
		return fmt.Errorf("git backend: worktree remove: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List enumerates the worktrees under root (filtering out the main repo and
// any worktree outside root).
func (b *Backend) List(root string) ([]worktree.Info, error) {
	ctx := context.Background()
	repo, err := b.repoDir(ctx)
	if err != nil {
		return nil, err
	}
	out, err := b.runner.Run(ctx, "-C", repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git backend: worktree list: %w", err)
	}
	return parseWorktreeList(string(out), root), nil
}

// Status summarises the worktree's working state from porcelain=v2.
func (b *Backend) Status(path string) (worktree.Status, error) {
	ctx := context.Background()
	out, err := b.runner.Run(ctx, "-C", path, "status", "--porcelain=v2", "-b")
	if err != nil {
		return worktree.Status{}, fmt.Errorf("git backend: status: %w", err)
	}
	return parseStatusV2(string(out)), nil
}

func (b *Backend) branchExists(ctx context.Context, repo, branch string) bool {
	_, err := b.runner.Run(ctx, "-C", repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// parseWorktreeList parses `git worktree list --porcelain`, returning only
// entries whose path is under root.
func parseWorktreeList(out, root string) []worktree.Info {
	var infos []worktree.Info
	var cur worktree.Info
	flush := func() {
		if cur.Path != "" && strings.HasPrefix(cur.Path, root) {
			cur.Backend = Name
			infos = append(infos, cur)
		}
		cur = worktree.Info{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "HEAD "):
			cur.HeadSHA = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "branch ")), "refs/heads/")
		case line == "":
			flush()
		}
	}
	flush()
	return infos
}

// parseStatusV2 parses `git status --porcelain=v2 -b` into a Status.
func parseStatusV2(out string) worktree.Status {
	st := worktree.Status{Clean: true}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "# branch.ab "):
			f := strings.Fields(line) // # branch.ab +A -B
			if len(f) >= 4 {
				st.Ahead = countField(f[2])
				st.Behind = countField(f[3])
			}
		case strings.HasPrefix(line, "#"):
			// other branch header; ignore
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			st.Clean = false
			f := strings.Fields(line)
			if len(f) >= 2 {
				xy := f[1]
				path := f[len(f)-1]
				if len(xy) == 2 {
					if xy[0] != '.' {
						st.Staged = append(st.Staged, path)
					}
					if xy[1] != '.' {
						st.Dirty = append(st.Dirty, path)
					}
				}
			}
		case strings.HasPrefix(line, "u "):
			st.Clean = false
			f := strings.Fields(line)
			st.Dirty = append(st.Dirty, f[len(f)-1])
		case strings.HasPrefix(line, "? "):
			st.Clean = false
			st.Untracked = append(st.Untracked, strings.TrimPrefix(line, "? "))
		}
	}
	return st
}

// countField parses a "+3" / "-2" ahead/behind token into a non-negative
// count.
func countField(s string) int {
	n, _ := strconv.Atoi(strings.TrimLeft(s, "+-"))
	return n
}
