package repo

import (
	"context"
	"errors"
	"testing"

	gitfake "github.com/wenlock/dotfiles/gsl/internal/git/fake"
)

func TestLocate_MainWorktree(t *testing.T) {
	// IsLinked → false (same git-dir and common-dir), Toplevel, Count
	r := &gitfake.Runner{
		Script: []gitfake.Response{
			// IsLinked: --git-dir
			{Stdout: []byte("/repo/.git\n")},
			// IsLinked: --git-common-dir
			{Stdout: []byte("/repo/.git\n")},
			// Toplevel
			{Stdout: []byte("/repo\n")},
			// Count (worktree list --porcelain)
			{Stdout: []byte("worktree /repo\nHEAD abc123\nbranch refs/heads/main\n")},
		},
	}

	loc, err := Locate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("Locate: unexpected error: %v", err)
	}
	if loc.IsWorktree {
		t.Error("IsWorktree: want false (main worktree), got true")
	}
	if loc.Toplevel != "/repo" {
		t.Errorf("Toplevel: want /repo, got %q", loc.Toplevel)
	}
	if loc.WorktreeCount != 1 {
		t.Errorf("WorktreeCount: want 1, got %d", loc.WorktreeCount)
	}
}

func TestLocate_LinkedWorktree(t *testing.T) {
	// IsLinked → true (git-dir != common-dir), Toplevel, Count=2
	r := &gitfake.Runner{
		Script: []gitfake.Response{
			// IsLinked: --git-dir (worktree-private)
			{Stdout: []byte("/repo/.git/worktrees/feat\n")},
			// IsLinked: --git-common-dir
			{Stdout: []byte("/repo/.git\n")},
			// Toplevel
			{Stdout: []byte("/some/worktree/path\n")},
			// Count (worktree list --porcelain) — 2 entries
			{Stdout: []byte("worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /some/worktree/path\nHEAD def\nbranch refs/heads/feat\n")},
		},
	}

	loc, err := Locate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("Locate: unexpected error: %v", err)
	}
	if !loc.IsWorktree {
		t.Error("IsWorktree: want true (linked worktree), got false")
	}
	if loc.Toplevel != "/some/worktree/path" {
		t.Errorf("Toplevel: want /some/worktree/path, got %q", loc.Toplevel)
	}
	if loc.WorktreeCount != 2 {
		t.Errorf("WorktreeCount: want 2, got %d", loc.WorktreeCount)
	}
}

func TestLocate_NotARepo(t *testing.T) {
	// IsLinked fails → ErrNotARepo returned
	r := &gitfake.Runner{
		Default: gitfake.Response{
			Err: errors.New("not a git repository"),
		},
	}

	_, err := Locate(context.Background(), r, "")
	if !errors.Is(err, ErrNotARepo) {
		t.Errorf("Locate: want ErrNotARepo, got %v", err)
	}
}

func TestLocate_ToplevelError(t *testing.T) {
	// IsLinked OK but Toplevel fails → ErrNotARepo
	r := &gitfake.Runner{
		Script: []gitfake.Response{
			// IsLinked: --git-dir
			{Stdout: []byte("/repo/.git\n")},
			// IsLinked: --git-common-dir
			{Stdout: []byte("/repo/.git\n")},
			// Toplevel fails
			{Err: errors.New("fatal: not a git repo")},
		},
	}

	_, err := Locate(context.Background(), r, "")
	if !errors.Is(err, ErrNotARepo) {
		t.Errorf("Locate: want ErrNotARepo, got %v", err)
	}
}

func TestLocate_CountError_NonFatal(t *testing.T) {
	// Count failing should not cause an error; count defaults to 1.
	r := &gitfake.Runner{
		Script: []gitfake.Response{
			// IsLinked: --git-dir
			{Stdout: []byte("/repo/.git\n")},
			// IsLinked: --git-common-dir
			{Stdout: []byte("/repo/.git\n")},
			// Toplevel
			{Stdout: []byte("/repo\n")},
			// Count fails
			{Err: errors.New("worktree list failed")},
		},
	}

	loc, err := Locate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("Locate: unexpected error: %v", err)
	}
	if loc.WorktreeCount != 1 {
		t.Errorf("WorktreeCount: want 1 (default on error), got %d", loc.WorktreeCount)
	}
}
