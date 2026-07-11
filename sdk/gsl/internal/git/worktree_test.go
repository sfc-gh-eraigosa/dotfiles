package git_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
)

// ---------------------------------------------------------------------------
// IsLinked
// ---------------------------------------------------------------------------

func TestIsLinkedTrue(t *testing.T) {
	// git-dir ≠ git-common-dir → linked worktree
	r := &fake.Runner{
		Script: []fake.Response{
			// rev-parse --git-dir → worktree-private path
			{Stdout: []byte("/repo/.git/worktrees/feat\n")},
			// rev-parse --git-common-dir → main .git
			{Stdout: []byte("/repo/.git\n")},
		},
	}

	linked, err := git.IsLinked(context.Background(), r, "/repo/.git/worktrees/feat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !linked {
		t.Error("IsLinked = false; want true for divergent git-dir/common-dir")
	}
}

func TestIsLinkedFalse(t *testing.T) {
	// git-dir == git-common-dir → main (canonical) worktree
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("/repo/.git\n")},
			{Stdout: []byte("/repo/.git\n")},
		},
	}

	linked, err := git.IsLinked(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked {
		t.Error("IsLinked = true; want false for identical git-dir/common-dir")
	}
}

func TestIsLinkedError(t *testing.T) {
	sentinel := errors.New("not a git repository")
	r := &fake.Runner{
		Default: fake.Response{Err: sentinel},
	}

	_, err := git.IsLinked(context.Background(), r, "/notgit")
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

const worktreePorcelainOne = `worktree /repo
HEAD abc123
branch refs/heads/main

`

const worktreePorcelainFour = `worktree /repo
HEAD abc123
branch refs/heads/main

worktree /repo/.git/worktrees/feat
HEAD def456
branch refs/heads/feat

worktree /repo/.git/worktrees/fix
HEAD ghi789
branch refs/heads/fix

worktree /repo/.git/worktrees/exp
HEAD jkl012
detached

`

func TestCountOne(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(worktreePorcelainOne)},
		},
	}

	n, err := git.Count(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("Count = %d; want 1", n)
	}
}

func TestCountFour(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(worktreePorcelainFour)},
		},
	}

	n, err := git.Count(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Errorf("Count = %d; want 4", n)
	}
}

func TestCountError(t *testing.T) {
	sentinel := errors.New("not a git repository")
	r := &fake.Runner{
		Default: fake.Response{Err: sentinel},
	}

	_, err := git.Count(context.Background(), r, "/notgit")
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Toplevel
// ---------------------------------------------------------------------------

func TestToplevel(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("/home/user/repo\n")},
		},
	}

	top, err := git.Toplevel(context.Background(), r, "/home/user/repo/subdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top != "/home/user/repo" {
		t.Errorf("Toplevel = %q; want '/home/user/repo'", top)
	}
}

func TestToplevelTrimsWhitespace(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			// trailing newline + spaces should be trimmed
			{Stdout: []byte("  /home/user/repo  \n")},
		},
	}

	top, err := git.Toplevel(context.Background(), r, "/home/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if top != "/home/user/repo" {
		t.Errorf("Toplevel = %q; want '/home/user/repo'", top)
	}
}

func TestToplevelError(t *testing.T) {
	sentinel := errors.New("not a git repository")
	r := &fake.Runner{
		Default: fake.Response{Err: sentinel},
	}

	_, err := git.Toplevel(context.Background(), r, "/notgit")
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// -C dir flag (Fix 2)
// ---------------------------------------------------------------------------

// TestDirFlagPrepended asserts that when a non-empty dir is passed to the git
// functions, the first Runner argument is "-C" followed by the dir. When dir
// is empty, "-C" must NOT appear.
func TestDirFlagPrepended(t *testing.T) {
	const dir = "/wt/feat"

	t.Run("IsLinked with dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte("/repo/.git/worktrees/feat\n")},
				{Stdout: []byte("/repo/.git\n")},
			},
		}
		if _, err := git.IsLinked(context.Background(), r, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.Calls) == 0 {
			t.Fatal("no calls recorded")
		}
		first := r.Calls[0]
		if first.Name != "-C" {
			t.Errorf("IsLinked with dir: Name = %q; want \"-C\"", first.Name)
		}
		if len(first.Args) == 0 || first.Args[0] != dir {
			t.Errorf("IsLinked with dir: Args[0] = %q; want %q", first.Args[0], dir)
		}
	})

	t.Run("IsLinked without dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte("/repo/.git\n")},
				{Stdout: []byte("/repo/.git\n")},
			},
		}
		if _, err := git.IsLinked(context.Background(), r, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if first := r.Calls[0]; first.Name == "-C" {
			t.Errorf("IsLinked without dir: Name = %q; should not be \"-C\"", first.Name)
		}
	})

	t.Run("Toplevel with dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte("/home/user/repo\n")},
			},
		}
		if _, err := git.Toplevel(context.Background(), r, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		first := r.Calls[0]
		if first.Name != "-C" {
			t.Errorf("Toplevel with dir: Name = %q; want \"-C\"", first.Name)
		}
		if len(first.Args) == 0 || first.Args[0] != dir {
			t.Errorf("Toplevel with dir: Args[0] = %q; want %q", first.Args[0], dir)
		}
	})

	t.Run("Toplevel without dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte("/home/user/repo\n")},
			},
		}
		if _, err := git.Toplevel(context.Background(), r, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if first := r.Calls[0]; first.Name == "-C" {
			t.Errorf("Toplevel without dir: Name = %q; should not be \"-C\"", first.Name)
		}
	})

	t.Run("Count with dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte(worktreePorcelainOne)},
			},
		}
		if _, err := git.Count(context.Background(), r, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		first := r.Calls[0]
		if first.Name != "-C" {
			t.Errorf("Count with dir: Name = %q; want \"-C\"", first.Name)
		}
		if len(first.Args) == 0 || first.Args[0] != dir {
			t.Errorf("Count with dir: Args[0] = %q; want %q", first.Args[0], dir)
		}
	})

	t.Run("Count without dir", func(t *testing.T) {
		r := &fake.Runner{
			Script: []fake.Response{
				{Stdout: []byte(worktreePorcelainOne)},
			},
		}
		if _, err := git.Count(context.Background(), r, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if first := r.Calls[0]; first.Name == "-C" {
			t.Errorf("Count without dir: Name = %q; should not be \"-C\"", first.Name)
		}
	})
}
