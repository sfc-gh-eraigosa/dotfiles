package render

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	mcpfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// update regenerates the golden files when set: `go test -run Golden -update`.
var update = flag.Bool("update", false, "update golden files")

// buildGoldenSegments wires the four real segments with deterministic fakes
// for the given location, glyph style, and a fixed clock + payload. Each
// runner is private to its segment so the concurrent render is deterministic.
//
// mcpDir is an isolated temp directory used ONLY for MCP detection (a
// .mcp.json with two servers + the active-count cache). The displayed working
// directory is a fixed "/home/user/project" so the dirgit basename is stable.
func buildGoldenSegments(t *testing.T, isWorktree bool, mcpDir string) []Segment {
	t.Helper()

	const displayCwd = "/home/user/project"

	// dirgit: scripted git.Status on branch "feature".
	dirRunner := &gitfake.Runner{Script: gitStatusResponses("feature")}
	dirSeg := &DirGitSegment{Cwd: displayCwd, Git: dirRunner, home: "/home/user"}

	// repo: scripted repo.Locate; registry supplies feature "gsl" + PR#21 OPEN.
	repoRunner := &gitfake.Runner{Script: locateResponses(isWorktree, "/wt/gsl", boolToCount(isWorktree))}
	repoSeg := NewRepoSegment(repoRunner, &ghfake.Runner{}, testBranch, registryPath(), nil)

	// ai: full payload; MCP configured=2 (.mcp.json in mcpDir), active=1 via fake.
	writeMcpJSON(t, mcpDir, 2)
	mcpRunner := &mcpfake.Runner{Default: mcpfake.Response{Stdout: []byte("a: x - ✓ Connected\nb: y - ✗ no\n")}}
	aiSeg := NewAISegment(samplePayload(), mcpDir, mcpRunner, mcp.ActiveCountOptions{CacheFile: filepath.Join(mcpDir, "ac.json")})

	// time: fixed clock in UTC for a stable abbreviation.
	timeSeg := NewTimeSegment(fixedClock(), "UTC", "15:04", "Mon 01-02")

	return []Segment{dirSeg, repoSeg, aiSeg, timeSeg}
}

func boolToCount(isWorktree bool) int {
	if isWorktree {
		return 2
	}
	return 1
}

func TestGolden(t *testing.T) {
	cases := []struct {
		name      string
		styleName string
		worktree  bool
	}{
		{"powerline_root", "powerline", false},
		{"powerline_worktree", "powerline", true},
		{"emoji_root", "emoji", false},
		{"emoji_worktree", "emoji", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Isolate MCP config + cache to a temp dir per case.
			dir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			t.Setenv("XDG_CACHE_HOME", dir)

			st := style.Resolve(discardWriter{}, tc.styleName, nil, false)
			segs := buildGoldenSegments(t, tc.worktree, dir)

			cfg := config.Default()
			got := Render(context.Background(), cfg, st, segs)
			if got == "" {
				t.Fatal("golden: rendered line is empty")
			}

			goldenPath := filepath.Join("testdata", "golden_"+tc.name+".txt")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("golden: write %s: %v", goldenPath, err)
				}
				return
			}

			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden: read %s: %v (run with -update to create)", goldenPath, err)
			}
			want := string(wantBytes)
			if got != want {
				t.Errorf("golden %s mismatch:\n got: %q\nwant: %q", tc.name, got, want)
			}
		})
	}
}

// TestGolden_SanityMarkers asserts the structural invariants of each golden
// combo independent of the exact byte string, so a future glyph tweak that
// legitimately regenerates the golden file still has to satisfy these.
func TestGolden_SanityMarkers(t *testing.T) {
	cases := []struct {
		name      string
		styleName string
		worktree  bool
		markers   []string
	}{
		{
			name: "powerline_root", styleName: "powerline", worktree: false,
			markers: []string{"project", "feature", "PR#21", "Opus 4.7", "14:30", "UTC"},
		},
		{
			name: "emoji_worktree", styleName: "emoji", worktree: true,
			markers: []string{"🌳", "project", "PR#21", "🤖", "⏰"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			t.Setenv("XDG_CACHE_HOME", dir)
			st := style.Resolve(discardWriter{}, tc.styleName, nil, false)
			segs := buildGoldenSegments(t, tc.worktree, dir)
			got := Render(context.Background(), config.Default(), st, segs)
			for _, m := range tc.markers {
				if !strings.Contains(got, m) {
					t.Errorf("%s: golden line missing marker %q\nline: %q", tc.name, m, got)
				}
			}
		})
	}
}
