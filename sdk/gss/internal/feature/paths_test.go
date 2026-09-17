// Internal tests for the WORKER.md path helper and legacy migration
// (issue #132). These live in `package feature` because migrateLegacyWorkerMD
// is unexported; WorkerMetaPath is exported and also exercised from the
// external feature_test package.
package feature

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
)

func TestWorkerMetaPath(t *testing.T) {
	cases := []struct {
		name     string
		worktree string
		want     string
	}{
		{
			name:     "plain leaf",
			worktree: "/root/wt/octo/proj/auth/erai/api",
			want:     "/root/wt/octo/proj/auth/erai/.gss-meta/api/WORKER.md",
		},
		{
			name:     "suffixed leaf keys on the full leaf base",
			worktree: "/root/wt/octo/proj/auth/erai/api-moss",
			want:     "/root/wt/octo/proj/auth/erai/.gss-meta/api-moss/WORKER.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkerMetaPath(tc.worktree); got != tc.want {
				t.Errorf("WorkerMetaPath(%q) = %q; want %q", tc.worktree, got, tc.want)
			}
		})
	}
}

// metaSiblingOfWorktree asserts the documented invariant: the meta file is
// OUTSIDE the worktree (never under it), which is the whole point of #132.
func TestWorkerMetaPath_OutsideWorktree(t *testing.T) {
	wt := "/root/wt/octo/proj/auth/erai/api"
	meta := WorkerMetaPath(wt)
	if rel, err := filepath.Rel(wt, meta); err == nil && !filepathEscapes(rel) {
		t.Errorf("meta path %q is INSIDE the worktree %q (rel=%q); #132 requires it outside", meta, wt, rel)
	}
}

// filepathEscapes reports whether a relative path steps out of its base
// (begins with ".."), i.e. the target is not contained by the base.
func filepathEscapes(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}

func TestMigrateLegacyWorkerMD_MovesUntracked(t *testing.T) {
	wt := t.TempDir()
	legacy := filepath.Join(wt, "WORKER.md")
	if err := os.WriteFile(legacy, []byte("# seeded\nhand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{Git: &gitfake.Runner{}}
	if err := s.migrateLegacyWorkerMD(context.Background(), wt); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy root WORKER.md still present (err=%v)", err)
	}
	got, err := os.ReadFile(WorkerMetaPath(wt))
	if err != nil {
		t.Fatalf("meta file not created: %v", err)
	}
	if string(got) != "# seeded\nhand edit\n" {
		t.Errorf("content not preserved: %q", got)
	}
}

func TestMigrateLegacyWorkerMD_NoLegacyIsNoOp(t *testing.T) {
	wt := t.TempDir()
	gitr := &gitfake.Runner{}
	s := &Service{Git: gitr}
	if err := s.migrateLegacyWorkerMD(context.Background(), wt); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(gitr.Calls) != 0 {
		t.Errorf("no legacy file → no git calls; got %+v", gitr.Calls)
	}
	if _, err := os.Stat(WorkerMetaPath(wt)); !os.IsNotExist(err) {
		t.Errorf("no-op migration must not create the meta file")
	}
}

func TestMigrateLegacyWorkerMD_TrackedFileDroppedFromIndex(t *testing.T) {
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, "WORKER.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitr := &gitfake.Runner{}
	s := &Service{Git: gitr}
	if err := s.migrateLegacyWorkerMD(context.Background(), wt); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A tracked legacy file must be removed from the index so it leaves
	// `git status`; --ignore-unmatch makes this a no-op when untracked.
	var sawRmCached bool
	for _, c := range gitr.Calls {
		if argsHas(c.Args, "rm") && argsHas(c.Args, "--cached") && argsHas(c.Args, "WORKER.md") {
			sawRmCached = true
		}
	}
	if !sawRmCached {
		t.Errorf("expected `git rm --cached -- WORKER.md`; calls=%+v", gitr.Calls)
	}
}

// argsHas is a tiny local contains-helper (the external test package has its
// own argsHasFC; this internal test needs its own).
func argsHas(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// seedWorkerMD writes a WORKER.md at worktree's meta path, creating parent
// dirs as needed.
func seedWorkerMD(t *testing.T, worktree string) {
	t.Helper()
	meta := WorkerMetaPath(worktree)
	if err := os.MkdirAll(filepath.Dir(meta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(meta, []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFindOrphanedWorkerMD pins the dotfiles#336 fix (ask #2): when cwd is
// not inside any registered worker per the registry (IsInWorker returned
// false), but a WORKER.md still exists at what would be the meta path for
// cwd (or an ancestor of cwd) — that's conclusive evidence this directory
// WAS a real worker root and its registry row went missing out from under
// it, rather than cwd simply never having been a worker at all. Nothing
// else removes WORKER.md when a row is dropped this way (only `gss
// feature done` removes it, deliberately, alongside the row).
func TestFindOrphanedWorkerMD(t *testing.T) {
	t.Run("cwd is the worktree root itself", func(t *testing.T) {
		root := t.TempDir()
		wt := filepath.Join(root, "auth", "erai", "api")
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		seedWorkerMD(t, wt)
		got, ok := FindOrphanedWorkerMD(wt)
		if !ok || got != wt {
			t.Errorf("FindOrphanedWorkerMD(%q) = (%q, %v); want (%q, true)", wt, got, ok, wt)
		}
	})

	t.Run("cwd is a subdirectory of the worktree root", func(t *testing.T) {
		root := t.TempDir()
		wt := filepath.Join(root, "auth", "erai", "api")
		sub := filepath.Join(wt, "internal", "deep")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		seedWorkerMD(t, wt)
		got, ok := FindOrphanedWorkerMD(sub)
		if !ok || got != wt {
			t.Errorf("FindOrphanedWorkerMD(%q) = (%q, %v); want (%q, true) (walks up to the worktree root)", sub, got, ok, wt)
		}
	})

	t.Run("no WORKER.md anywhere up the tree", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "just", "a", "normal", "directory")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, ok := FindOrphanedWorkerMD(dir); ok {
			t.Error("no WORKER.md anywhere: want (_, false)")
		}
	})

	t.Run("empty cwd", func(t *testing.T) {
		if _, ok := FindOrphanedWorkerMD(""); ok {
			t.Error(`FindOrphanedWorkerMD(""): want (_, false)`)
		}
	})
}
