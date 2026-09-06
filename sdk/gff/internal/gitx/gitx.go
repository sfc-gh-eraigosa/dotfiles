// Package gitx provides git repo discovery and a mockable runner interface.
package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner is the interface for running git sub-commands.
// It is mockable in tests via fakeRunner.
type Runner interface {
	Output(dir string, args ...string) (string, error)
}

// ExecRunner is the real Runner implementation that shells out to git.
type ExecRunner struct{}

// Output runs `git -C <dir> <args...>` and returns the combined stdout, trimmed.
func (ExecRunner) Output(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RepoRoot walks up the directory tree from startDir, looking for a .git entry
// (either a directory or a file, the latter being the git worktree case).
// Returns the directory that contains .git and true; or ("", false) if not found.
func RepoRoot(startDir string) (string, bool) {
	dir := startDir
	for {
		_, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root.
			return "", false
		}
		dir = parent
	}
}

// SourcePath resolves the path to a repo's gff feature file.
//
// Resolution order:
//  1. `git config --get gff.source` — if the runner returns a non-empty string,
//     treat it as the source path. Relative paths are joined to repoRoot.
//  2. Probe .gff/features.yaml — if it exists, return it.
//  3. Probe .github/gff/features.yaml — if it exists, return it.
//  4. Fall back to .gff/features.yaml (the live layer will simply be absent).
func SourcePath(r Runner, repoRoot string) string {
	out, err := r.Output(repoRoot, "config", "--get", "gff.source")
	if err == nil && out != "" {
		if filepath.IsAbs(out) {
			return out
		}
		return filepath.Join(repoRoot, out)
	}

	// Probe order: .gff wins over .github/gff.
	primary := filepath.Join(repoRoot, ".gff", "features.yaml")
	if _, statErr := os.Stat(primary); statErr == nil {
		return primary
	}

	secondary := filepath.Join(repoRoot, ".github", "gff", "features.yaml")
	if _, statErr := os.Stat(secondary); statErr == nil {
		return secondary
	}

	// Neither exists; return the primary path (resolver treats missing file as absent).
	return primary
}
