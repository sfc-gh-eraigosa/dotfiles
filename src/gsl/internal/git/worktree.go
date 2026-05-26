package git

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"time"
)

const worktreeTimeout = 800 * time.Millisecond

// IsLinked reports whether dir is a linked worktree (as opposed to the
// canonical "main" worktree / repo root).
//
// Detection: compare `git rev-parse --path-format=absolute --git-dir` with
// `git rev-parse --path-format=absolute --git-common-dir`. In the main
// worktree both paths resolve to the same directory (e.g. /repo/.git).
// In a linked worktree --git-dir points to a worktree-private directory
// (e.g. /repo/.git/worktrees/feat) while --git-common-dir still points to
// /repo/.git — so they differ.
func IsLinked(ctx context.Context, r Runner, _ string) (bool, error) {
	tctx, cancel := context.WithTimeout(ctx, worktreeTimeout)
	defer cancel()

	// Two separate rev-parse calls so we can compare the values.
	gitDir, err := revParseSingle(tctx, r, "--path-format=absolute", "--git-dir")
	if err != nil {
		return false, err
	}
	commonDir, err := revParseSingle(tctx, r, "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false, err
	}

	return gitDir != commonDir, nil
}

// Count returns the number of worktrees (including the main one) by counting
// "worktree " header lines in `git worktree list --porcelain` output.
func Count(ctx context.Context, r Runner, _ string) (int, error) {
	tctx, cancel := context.WithTimeout(ctx, worktreeTimeout)
	defer cancel()

	out, err := r.Run(tctx, "worktree", "list", "--porcelain")
	if err != nil {
		return 0, err
	}

	return countWorktreeEntries(out), nil
}

// Toplevel returns the absolute path of the root of the working tree,
// i.e. `git rev-parse --show-toplevel` (trimmed).
func Toplevel(ctx context.Context, r Runner, _ string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, worktreeTimeout)
	defer cancel()

	return revParseSingle(tctx, r, "--show-toplevel")
}

// revParseSingle runs `git rev-parse <args...>` and returns the first
// non-empty, trimmed output line.
func revParseSingle(ctx context.Context, r Runner, args ...string) (string, error) {
	out, err := r.Run(ctx, "rev-parse", args...)
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}

// countWorktreeEntries counts "worktree " prefix lines in porcelain output.
func countWorktreeEntries(out []byte) int {
	count := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "worktree ") {
			count++
		}
	}
	return count
}
