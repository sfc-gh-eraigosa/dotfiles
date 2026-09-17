package feature

import (
	"context"
	"os"
	"path/filepath"
)

// WorkerMetaPath returns the on-disk location of a worker's WORKER.md, which
// lives OUTSIDE the worker's git worktree so it can never appear in the
// consumer repo's `git status` or be accidentally committed (issue #132).
//
// The path is leaf-keyed under a `.gss-meta/` namespace one level above the
// worktree:
//
//	<feature>/<user>/.gss-meta/<leaf>/WORKER.md
//
// where <leaf> == filepath.Base(worktree) and <feature>/<user>/ ==
// filepath.Dir(worktree). Keying on the leaf (not the shared <feature>/<user>/
// parent) is what makes sibling workers collision-safe: `design/` and `impl/`
// for the same user get distinct files. It derives solely from the worktree
// path, so every touchpoint (seed write, auto-log append, teardown) agrees
// without a registry-schema change.
func WorkerMetaPath(worktree string) string {
	return filepath.Join(filepath.Dir(worktree), ".gss-meta", filepath.Base(worktree), "WORKER.md")
}

// FindOrphanedWorkerMD walks upward from cwd looking for a directory whose
// WORKER.md meta path (WorkerMetaPath) exists on disk, even though the
// caller has already established cwd is not inside any REGISTERED worker
// (mode.IsInWorker returned false). A hit is conclusive evidence the
// directory WAS created as a real worker root — the only thing that
// removes WORKER.md is `gss feature done`, which removes it deliberately
// alongside the registry row — so if the row is gone but this file
// survives, the row went missing out from under a live worker rather than
// cwd simply never having been one (dotfiles#336: a cross-repo `audit
// --repair` run is a confirmed cause). Returns the worktree-root candidate
// and true on a hit; ("", false) when cwd carries no such trace at all the
// way up to the filesystem root.
func FindOrphanedWorkerMD(cwd string) (string, bool) {
	dir := cwd
	for dir != "" {
		if _, err := os.Stat(WorkerMetaPath(dir)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return "", false
}

// workerMetaDir is the per-leaf directory holding WORKER.md; teardown removes
// it wholesale (done.go).
func workerMetaDir(worktree string) string {
	return filepath.Join(filepath.Dir(worktree), ".gss-meta", filepath.Base(worktree))
}

// migrateLegacyWorkerMD relocates a pre-existing root-level WORKER.md (seeded
// by an older gss into the worktree itself) to the new meta path, so upgrading
// gss does not strand the legacy file in `git status`. It is idempotent: a
// no-op when no legacy file exists.
//
// Content is preserved (os.Rename, not rewrite). If the legacy file had been
// committed, it is dropped from the index with `git rm --cached` so it leaves
// `git status`; --ignore-unmatch makes that a no-op for the common (untracked)
// case. worker add always creates a FRESH worktree, so the only path that ever
// encounters a legacy file is auto-checkpoint over an existing worktree —
// hence this runs at AutoCheckpoint entry.
func (s *Service) migrateLegacyWorkerMD(ctx context.Context, worktree string) error {
	legacy := filepath.Join(worktree, "WORKER.md")
	if _, err := os.Stat(legacy); err != nil {
		return nil // no legacy file → nothing to migrate
	}
	meta := WorkerMetaPath(worktree)
	if _, err := os.Stat(meta); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(meta), 0o755); err != nil {
			return err
		}
		if err := os.Rename(legacy, meta); err != nil {
			return err
		}
	} else {
		// meta already authoritative → drop the stray legacy working copy.
		_ = os.Remove(legacy)
	}
	// Clear it from the index if it was ever committed (no-op when untracked).
	_, _ = s.Git.Run(ctx, "-C", worktree, "rm", "--cached", "--ignore-unmatch", "--quiet", "--", "WORKER.md")
	return nil
}
