// Package scan implements `gss scan`: walk a directory tree and report git
// repos with uncommitted changes (design.md → "Existing features that must
// survive the refactor"). The "[DIRTY] <path>" output line is a stable
// contract grepped by slash commands, so Format reproduces it byte-for-byte
// from the classic cmd/scan.go.
//
// Dirtiness is determined through an injected IsDirty func so the walk is
// unit-testable without real git repos; GitDirty wires the production
// implementation over a git.Runner.
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/git"
)

// Scanner walks a tree and classifies the git repos it finds.
type Scanner struct {
	// IsDirty reports whether the repo rooted at dir has uncommitted
	// changes. Required.
	IsDirty func(dir string) bool
}

// Scan walks root depth-first (lexical order) and returns the paths of
// dirty repos. A repo is any directory containing a ".git" subdirectory —
// matching the classic command (worktree ".git" *files* are intentionally
// not matched). The walk does not descend into ".git" itself, but does
// descend past a repo into nested repos. filepath.WalkDir does not follow
// symlinks, so symlink loops cannot hang the walk. Per-entry errors are
// skipped, mirroring the classic behaviour.
func (s *Scanner) Scan(root string) ([]string, error) {
	var dirty []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() && d.Name() == ".git" {
			repo := filepath.Dir(path)
			if s.IsDirty(repo) {
				dirty = append(dirty, repo)
			}
			return fs.SkipDir // never descend into .git
		}
		return nil
	})
	return dirty, err
}

// Format renders the dirty-repo report exactly as the classic command:
// one "[DIRTY] <path>\n" line per repo, in the order given.
func Format(dirty []string) string {
	var b strings.Builder
	for _, p := range dirty {
		fmt.Fprintf(&b, "[DIRTY] %s\n", p)
	}
	return b.String()
}

// GitDirty returns an IsDirty backed by a git.Runner: `git status
// --porcelain` with non-empty (trimmed) output means dirty. Any git error
// is treated as not-dirty, matching the classic command.
func GitDirty(ctx context.Context, runner git.Runner) func(string) bool {
	return func(dir string) bool {
		out, err := runner.Run(ctx, "-C", dir, "status", "--porcelain")
		if err != nil {
			return false
		}
		return len(strings.TrimSpace(string(out))) > 0
	}
}
