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
