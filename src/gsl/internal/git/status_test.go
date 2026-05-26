package git_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/git"
	"github.com/wenlock/dotfiles/gsl/internal/git/fake"
)

// porcelainV2Mixed is a realistic `git status --porcelain=v2 --branch`
// output with staged, unstaged, untracked, renamed, and conflict entries.
const porcelainV2Mixed = `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -1
1 M. N... 100644 100644 100644 aaa bbb file1.go
1 .M N... 100644 100644 100644 ccc ddd file2.go
1 MM N... 100644 100644 100644 eee fff file3.go
2 R. N... 100644 100644 100644 ggg hhh R100 newname.go\toldname.go
? untracked1.txt
? untracked2.txt
u UU N... 100644 100644 100644 100644 iii jjj kkk conflict.go
`

const stashListNonEmpty = `stash@{0}: WIP on main: abc123 some work
stash@{1}: WIP on feat: def456 other work
`

func TestStatusMixedRepo(t *testing.T) {
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(porcelainV2Mixed)},
			{Stdout: []byte(stashListNonEmpty)},
		},
	}

	info, err := git.Status(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Staged: file1 (X=M), file3 (X=M), renamed newname (X=R) → 3
	if info.Staged != 3 {
		t.Errorf("Staged = %d; want 3", info.Staged)
	}
	// Unstaged: file2 (Y=M), file3 (Y=M) → 2
	if info.Unstaged != 2 {
		t.Errorf("Unstaged = %d; want 2", info.Unstaged)
	}
	// Untracked: untracked1, untracked2 → 2
	if info.Untracked != 2 {
		t.Errorf("Untracked = %d; want 2", info.Untracked)
	}
	// Conflicts: conflict.go → 1
	if info.Conflicts != 1 {
		t.Errorf("Conflicts = %d; want 1", info.Conflicts)
	}
	// Ahead/behind from branch.ab
	if info.Ahead != 2 {
		t.Errorf("Ahead = %d; want 2", info.Ahead)
	}
	if info.Behind != 1 {
		t.Errorf("Behind = %d; want 1", info.Behind)
	}
	// Branch name
	if info.Branch != "main" {
		t.Errorf("Branch = %q; want 'main'", info.Branch)
	}
	if info.Detached {
		t.Error("Detached = true; want false")
	}
	// Stashes from stash list (2 lines)
	if info.Stashes != 2 {
		t.Errorf("Stashes = %d; want 2", info.Stashes)
	}
}

func TestStatusCleanRepo(t *testing.T) {
	cleanOutput := `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(cleanOutput)},
			{Stdout: nil}, // empty stash
		},
	}

	info, err := git.Status(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Staged != 0 || info.Unstaged != 0 || info.Untracked != 0 ||
		info.Conflicts != 0 || info.Ahead != 0 || info.Behind != 0 || info.Stashes != 0 {
		t.Errorf("clean repo: expected all-zero counts, got %+v", info)
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q; want 'main'", info.Branch)
	}
}

func TestStatusNotARepo(t *testing.T) {
	sentinel := errors.New("not a git repository")
	r := &fake.Runner{
		Default: fake.Response{Err: sentinel},
	}

	_, err := git.Status(context.Background(), r, "/notgit")
	if err == nil {
		t.Fatal("expected error for non-repo, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestStatusDetachedHead(t *testing.T) {
	detachedOutput := `# branch.oid deadbeef
# branch.head (detached)
`
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(detachedOutput)},
			{Stdout: nil},
		},
	}

	info, err := git.Status(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Detached {
		t.Error("Detached = false; want true")
	}
	if info.Branch != "(detached)" {
		t.Errorf("Branch = %q; want '(detached)'", info.Branch)
	}
}

func TestStatusTimeout(t *testing.T) {
	// Use an already-cancelled context to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &fake.Runner{
		Default: fake.Response{Stdout: []byte("should not be parsed")},
	}

	_, err := git.Status(ctx, r, "/repo")
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestStatusNoUpstream(t *testing.T) {
	// When no upstream is configured, branch.ab line is absent.
	noUpstream := `# branch.oid abc123
# branch.head feature
`
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(noUpstream)},
			{Stdout: nil},
		},
	}

	info, err := git.Status(context.Background(), r, "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("no-upstream: Ahead=%d Behind=%d; want both 0", info.Ahead, info.Behind)
	}
	if info.Branch != "feature" {
		t.Errorf("Branch = %q; want 'feature'", info.Branch)
	}
}
