// Package git_test runs the shared backend contract suite (PR-20) against
// the real git backend, plus verifies it sets rebase.updateRefs on each new
// worktree (src/gss/docs/plan.md PR-21). These tests shell out to a real
// git and skip cleanly when git isn't on PATH.
package git_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitrun "github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/worktree"
	"github.com/wenlock/dotfiles/gss/internal/worktree/backendtest"
	wtgit "github.com/wenlock/dotfiles/gss/internal/worktree/git"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(full, " "), err, out)
	}
}

// initMainRepo creates a temp repo on branch main with one commit.
func initMainRepo(t *testing.T) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	runGit(t, "", "init", "-b", "main", dir)
	runGit(t, dir, "config", "user.email", "gss-test@example.invalid")
	runGit(t, dir, "config", "user.name", "gss test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func TestGitBackend_Name(t *testing.T) {
	if (&wtgit.Backend{}).Name() != "git" {
		t.Errorf("Name() = %q; want git", (&wtgit.Backend{}).Name())
	}
}

func TestGitBackend_Contract(t *testing.T) {
	skipIfNoGit(t)
	backendtest.RunContractSuite(t, func(t *testing.T) backendtest.Fixture {
		repo := initMainRepo(t)
		root := t.TempDir()
		be := wtgit.NewBackend(repo, gitrun.NewSystemRunner())
		n := 0
		return backendtest.Fixture{
			Backend: be,
			Root:    root,
			NewReq: func() worktree.CreateReq {
				n++
				return worktree.CreateReq{
					Path:       filepath.Join(root, fmt.Sprintf("wt%d", n)),
					Branch:     fmt.Sprintf("feature/x/erai/p%d", n),
					BaseBranch: "main",
				}
			},
			MakeDirty: func(path string) {
				_ = os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("x"), 0o644)
			},
		}
	})
}

func TestGitBackend_SetsUpdateRefs(t *testing.T) {
	repo := initMainRepo(t)
	root := t.TempDir()
	be := wtgit.NewBackend(repo, gitrun.NewSystemRunner())

	wtPath := filepath.Join(root, "wt")
	info, err := be.Create(worktree.CreateReq{Path: wtPath, Branch: "feature/x/erai/api", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Backend != "git" || info.HeadSHA == "" {
		t.Errorf("Info = %+v; want Backend=git and a HeadSHA", info)
	}

	out, err := exec.Command("git", "-C", wtPath, "config", "rebase.updateRefs").Output()
	if err != nil {
		t.Fatalf("read rebase.updateRefs: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "true" {
		t.Errorf("rebase.updateRefs = %q; want true", got)
	}
}

func TestGitBackend_RegisteredViaInit(t *testing.T) {
	b, err := worktree.Open("git")
	if err != nil {
		t.Fatalf("Open(git): %v (init should have registered it)", err)
	}
	if b.Name() != "git" {
		t.Errorf("Name() = %q; want git", b.Name())
	}
}

func TestGitBackend_StatusReflectsDirty(t *testing.T) {
	repo := initMainRepo(t)
	be := wtgit.NewBackend(repo, gitrun.NewSystemRunner())
	wtPath := filepath.Join(t.TempDir(), "wt")
	if _, err := be.Create(worktree.CreateReq{Path: wtPath, Branch: "feature/x/erai/ui", BaseBranch: "main"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := be.Status(wtPath)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Clean {
		t.Errorf("fresh worktree not clean: %+v", st)
	}

	_ = os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("x"), 0o644)
	st, err = be.Status(wtPath)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Clean || len(st.Untracked) == 0 {
		t.Errorf("after writing untracked file, Status = %+v; want not-clean with untracked", st)
	}
}
