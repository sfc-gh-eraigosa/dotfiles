// Package scan_test verifies the dirty-repo walker per sdk/gss/docs/plan.md
// PR-13: recursive walk (incl. nested repos), symlink-loop safety, and the
// exact "[DIRTY] " output contract.
//
// Note: the test trees are built at runtime under t.TempDir() rather than
// committed as testdata/scan/... because git refuses to track paths inside
// a literal ".git" directory — so committed ".git" fixtures are impossible.
// Dirtiness is injected (decoupled from real git) for deterministic output.
package scan_test

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/scan"
)

// mkRepo creates <root>/<rel>/.git so the walker sees a repo at <root>/<rel>.
func mkRepo(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel, ".git"), 0o755); err != nil {
		t.Fatalf("mkRepo %s: %v", rel, err)
	}
}

// buildTree lays out clean/dirty/nested repos and returns the root.
func buildTree(t *testing.T) string {
	root := t.TempDir()
	mkRepo(t, root, "clean/repoA")        // clean
	mkRepo(t, root, "dirty/repoB")        // dirty
	mkRepo(t, root, "nested/outer")       // dirty (and contains a nested repo)
	mkRepo(t, root, "nested/outer/inner") // clean
	return root
}

// dirtyByBase marks repos dirty by their leaf directory name.
func dirtyByBase(names ...string) func(string) bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return func(dir string) bool { return set[filepath.Base(dir)] }
}

func TestScan_FindsDirtyReposIncludingNested(t *testing.T) {
	root := buildTree(t)
	s := &scan.Scanner{IsDirty: dirtyByBase("repoB", "outer")}

	got, err := s.Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// Make paths comparable across machines.
	var rel []string
	for _, p := range got {
		r, _ := filepath.Rel(root, p)
		rel = append(rel, filepath.ToSlash(r))
	}
	want := []string{"dirty/repoB", "nested/outer"}
	if len(rel) != len(want) {
		t.Fatalf("dirty repos = %v; want %v", rel, want)
	}
	for i := range want {
		if rel[i] != want[i] {
			t.Errorf("dirty[%d] = %q; want %q (order matters)", i, rel[i], want[i])
		}
	}
}

func TestScan_DescendsIntoNestedRepoWhenDirty(t *testing.T) {
	root := buildTree(t)
	// Mark the nested inner repo dirty too: walk must reach it.
	s := &scan.Scanner{IsDirty: dirtyByBase("inner")}
	got, err := s.Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "inner" {
		t.Errorf("got %v; want the nested inner repo", got)
	}
}

func TestScan_SymlinkLoopDoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on Windows CI")
	}
	root := t.TempDir()
	mkRepo(t, root, "repo")
	// A symlink pointing back at root would loop a follow-the-link walk;
	// filepath.WalkDir does not follow it, so Scan must return normally.
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	s := &scan.Scanner{IsDirty: func(string) bool { return true }}
	got, err := s.Scan(root)
	if err != nil {
		t.Fatalf("Scan with symlink loop: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d repos; want 1 (loop must not multiply or hang)", len(got))
	}
}

func TestFormat_DirtyContract(t *testing.T) {
	out := scan.Format([]string{"/a/repoB", "/b/outer"})
	want := "[DIRTY] /a/repoB\n[DIRTY] /b/outer\n"
	if out != want {
		t.Errorf("Format() = %q; want %q", out, want)
	}
	if scan.Format(nil) != "" {
		t.Errorf("Format(nil) = %q; want empty", scan.Format(nil))
	}
}

func TestGitDirty(t *testing.T) {
	clean := scan.GitDirty(t.Context(), &gitfake.Runner{Default: gitfake.Response{Stdout: []byte("")}})
	if clean("/repo") {
		t.Error("empty porcelain → should be clean")
	}
	dirty := scan.GitDirty(t.Context(), &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(" M file.go\n")}})
	if !dirty("/repo") {
		t.Error("non-empty porcelain → should be dirty")
	}
	errd := scan.GitDirty(t.Context(), &gitfake.Runner{Default: gitfake.Response{Err: stderrors.New("not a repo")}})
	if errd("/repo") {
		t.Error("git error → should be treated as not-dirty (classic behaviour)")
	}
}
