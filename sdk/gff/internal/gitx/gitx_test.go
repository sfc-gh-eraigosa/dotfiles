package gitx_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner is a test double for gitx.Runner.
type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Output(dir string, args ...string) (string, error) {
	return f.out, f.err
}

// TestRepoRootGitDir verifies that RepoRoot finds the root when .git is a directory.
func TestRepoRootGitDir(t *testing.T) {
	base := t.TempDir()
	// Create a/b/c — .git dir lives at a.
	aDir := filepath.Join(base, "a")
	cDir := filepath.Join(aDir, "b", "c")
	require.NoError(t, os.MkdirAll(cDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(aDir, ".git"), 0o755))

	root, ok := gitx.RepoRoot(cDir)
	assert.True(t, ok, "should find repo root from subdirectory")
	assert.Equal(t, aDir, root)
}

// TestRepoRootGitFile verifies that RepoRoot finds the root when .git is a file (worktree case).
func TestRepoRootGitFile(t *testing.T) {
	base := t.TempDir()
	aDir := filepath.Join(base, "a")
	cDir := filepath.Join(aDir, "b", "c")
	require.NoError(t, os.MkdirAll(cDir, 0o755))
	// Write a .git FILE — this is the worktree case.
	require.NoError(t, os.WriteFile(filepath.Join(aDir, ".git"), []byte("gitdir: ../.git/worktrees/foo\n"), 0o644))

	root, ok := gitx.RepoRoot(cDir)
	assert.True(t, ok, "should find repo root when .git is a file")
	assert.Equal(t, aDir, root)
}

// TestRepoRootNotFound verifies that RepoRoot returns ("", false) when there is no .git.
func TestRepoRootNotFound(t *testing.T) {
	base := t.TempDir()
	// No .git anywhere.
	root, ok := gitx.RepoRoot(base)
	assert.False(t, ok, "should return false when no .git found")
	assert.Equal(t, "", root)
}

// TestSourcePathRedirect verifies that a custom path from git config wins over probing.
func TestSourcePathRedirect(t *testing.T) {
	base := t.TempDir()
	runner := fakeRunner{out: "custom/flags.yaml"}

	result := gitx.SourcePath(runner, base)
	assert.Equal(t, filepath.Join(base, "custom/flags.yaml"), result,
		"relative redirect should be joined to repoRoot")
}

// TestSourcePathRedirectAbsolute verifies that an absolute path from git config is returned as-is.
func TestSourcePathRedirectAbsolute(t *testing.T) {
	base := t.TempDir()
	abs := "/some/absolute/path/flags.yaml"
	runner := fakeRunner{out: abs}

	result := gitx.SourcePath(runner, base)
	assert.Equal(t, abs, result, "absolute redirect should be returned unchanged")
}

// TestSourcePathProbeGffOnly verifies probe order: only .gff/features.yaml present => that path.
func TestSourcePathProbeGffOnly(t *testing.T) {
	base := t.TempDir()
	gffPath := filepath.Join(base, ".gff", "features.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gffPath), 0o755))
	require.NoError(t, os.WriteFile(gffPath, []byte(""), 0o644))

	runner := fakeRunner{err: errors.New("exit status 1")}
	result := gitx.SourcePath(runner, base)
	assert.Equal(t, gffPath, result, "should return .gff/features.yaml when it exists")
}

// TestSourcePathProbeGithubOnly verifies probe order: only .github/gff/features.yaml present => that path.
func TestSourcePathProbeGithubOnly(t *testing.T) {
	base := t.TempDir()
	githubPath := filepath.Join(base, ".github", "gff", "features.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(githubPath), 0o755))
	require.NoError(t, os.WriteFile(githubPath, []byte(""), 0o644))

	runner := fakeRunner{err: errors.New("exit status 1")}
	result := gitx.SourcePath(runner, base)
	assert.Equal(t, githubPath, result, "should return .github/gff/features.yaml when only it exists")
}

// TestSourcePathProbeBothPresent verifies that .gff/features.yaml wins when both exist.
func TestSourcePathProbeBothPresent(t *testing.T) {
	base := t.TempDir()
	gffPath := filepath.Join(base, ".gff", "features.yaml")
	githubPath := filepath.Join(base, ".github", "gff", "features.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(gffPath), 0o755))
	require.NoError(t, os.WriteFile(gffPath, []byte(""), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(githubPath), 0o755))
	require.NoError(t, os.WriteFile(githubPath, []byte(""), 0o644))

	runner := fakeRunner{err: errors.New("exit status 1")}
	result := gitx.SourcePath(runner, base)
	assert.Equal(t, gffPath, result, ".gff/features.yaml should win over .github/gff/features.yaml")
}

// TestSourcePathProbeNeither verifies that .gff/features.yaml is returned when neither file exists.
func TestSourcePathProbeNeither(t *testing.T) {
	base := t.TempDir()
	// Neither file exists.
	runner := fakeRunner{err: errors.New("exit status 1")}
	result := gitx.SourcePath(runner, base)
	expected := filepath.Join(base, ".gff", "features.yaml")
	assert.Equal(t, expected, result, "should return .gff/features.yaml (missing live layer) when neither exists")
}
