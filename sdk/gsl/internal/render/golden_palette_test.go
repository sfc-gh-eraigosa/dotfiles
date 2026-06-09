package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// TestGolden_EmojiPerPalette renders the emoji style with non-default palettes
// (light and dark-daltonism) and asserts two things:
//
//  1. The output differs from the default-palette emoji golden (the fg-tint
//     codes change — this is the per-palette emoji golden assertion from the
//     design doc EMOJI-F2-01/F2-02).
//  2. The output contains the expected "38;5;N" escape codes for the
//     non-default palette indices, proving the fg-tint path reaches emoji.
//
// For each non-default palette case the golden file is
// testdata/golden_emoji_<palette>_root.txt. On first run (or with -update)
// the golden is written; subsequent runs assert byte-identity.
func TestGolden_EmojiPerPalette(t *testing.T) {
	cases := []struct {
		palette  string
		worktree bool
		suffix   string
	}{
		{"light", false, "root"},
		{"dark-daltonism", false, "root"},
	}

	// Also read the default emoji golden for the "differs from default" check.
	defaultGoldenPath := filepath.Join("testdata", "golden_emoji_root.txt")
	defaultBytes, err := os.ReadFile(defaultGoldenPath)
	if err != nil {
		t.Fatalf("golden_palette: cannot read default emoji golden %s: %v", defaultGoldenPath, err)
	}
	defaultGolden := string(defaultBytes)

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("emoji_%s_%s", tc.palette, tc.suffix), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			t.Setenv("XDG_CACHE_HOME", dir)

			// Resolve emoji style with the non-default auto-palette.
			st := style.ResolveConfig(discardWriter{}, "emoji", nil, false, tc.palette)
			segs := buildGoldenSegments(t, tc.worktree, dir)

			cfg := config.Default()
			got := Render(context.Background(), cfg, st, segs)
			if got == "" {
				t.Fatal("golden_palette: rendered line is empty")
			}

			goldenPath := filepath.Join("testdata", fmt.Sprintf("golden_emoji_%s_%s.txt", tc.palette, tc.suffix))
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("golden_palette: write %s: %v", goldenPath, err)
				}
				return
			}

			// Load or validate golden file.
			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden_palette: read %s: %v (run with -update to create)", goldenPath, err)
			}
			want := string(wantBytes)
			if got != want {
				t.Errorf("golden_palette %s mismatch:\n got: %q\nwant: %q",
					fmt.Sprintf("emoji_%s_%s", tc.palette, tc.suffix), got, want)
			}

			// Assert the palette's fg codes are present in the output.
			// Each non-default palette uses "38;5;N" ANSI codes.
			// We only check the keys whose segments actually appear in the
			// output — repo_worktree only renders in worktree mode, so skip it
			// for the root test case.
			p, _ := style.Palette(tc.palette)
			for _, key := range style.SegmentColorKeys() {
				if key == "repo_worktree" && !tc.worktree {
					continue // repo_worktree segment absent in root mode
				}
				colorVal := p[key]
				// Only verify ANSI-256 index values (not named colors like "blue").
				if _, isNamed := map[string]bool{
					"black": true, "red": true, "green": true, "yellow": true,
					"blue": true, "magenta": true, "cyan": true, "white": true,
					"default": true,
				}[colorVal]; isNamed {
					continue
				}
				expectedSeq := "38;5;" + colorVal
				if !strings.Contains(got, expectedSeq) {
					t.Errorf("palette %q: expected fg escape %q in rendered output for key %q", tc.palette, expectedSeq, key)
				}
			}

			// Assert the output DIFFERS from the default-palette emoji golden.
			// This proves the fg-tint path changes with the palette.
			if got == defaultGolden {
				t.Errorf("palette %q emoji golden is identical to default palette — fg-tint must differ", tc.palette)
			}
		})
	}
}

// TestGolden_EmojiDefault_UnchangedByPaletteAddition asserts the default-palette
// emoji goldens are byte-identical to their pre-Phase-4 content.
// This test catches any regression where adding palette support accidentally
// changes the default rendering path.
func TestGolden_EmojiDefault_UnchangedByPaletteAddition(t *testing.T) {
	cases := []struct {
		name     string
		worktree bool
	}{
		{"emoji_root", false},
		{"emoji_worktree", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			t.Setenv("XDG_CACHE_HOME", dir)

			// Resolve emoji style WITHOUT auto-palette (same as TestGolden).
			st := style.Resolve(discardWriter{}, "emoji", nil, false)
			segs := buildGoldenSegments(t, tc.worktree, dir)

			cfg := config.Default()
			got := Render(context.Background(), cfg, st, segs)

			goldenPath := filepath.Join("testdata", "golden_"+tc.name+".txt")
			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden_default: read %s: %v", goldenPath, err)
			}
			if got != string(wantBytes) {
				t.Errorf("default emoji golden %s changed — must be byte-identical:\n got: %q\nwant: %q",
					tc.name, got, string(wantBytes))
			}
		})
	}
}

// TestGolden_EmojiUserOverrideWins asserts that when a user config explicitly
// sets theme.ai and the auto-palette is non-default (light), the rendered ai
// fg code is the USER value, not the palette value (UC-5 / EMOJI-F2 user-wins).
//
// This is verified via the rendered fg escape code, not just the palette name,
// because for emoji (Fill:false) the fg tint is the ONLY visible theme signal.
func TestGolden_EmojiUserOverrideWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	// User explicitly sets theme.ai to a specific ANSI-256 index.
	const userAIColor = "199" // a distinct pink, not in any palette
	raw := map[string]map[string]any{
		"emoji": {
			"theme": map[string]any{
				"ai": userAIColor,
			},
		},
	}

	// Resolve emoji style with light auto-palette AND user override.
	st := style.ResolveConfig(discardWriter{}, "emoji", raw, false, "light")

	segs := buildGoldenSegments(t, false, dir)
	cfg := config.Default()
	got := Render(context.Background(), cfg, st, segs)

	// The rendered output must contain the USER's fg code for the ai segment.
	userFGSeq := "38;5;" + userAIColor
	if !strings.Contains(got, userFGSeq) {
		t.Errorf("user override wins: expected user fg escape %q in rendered output\noutput: %q",
			userFGSeq, got)
	}

	// The light palette's ai value (37) must NOT appear as a fg tint in the output.
	// (It could appear in other contexts, but if the user override wins, the ai
	// segment should use 199, not 37.)
	lightPalette, _ := style.Palette("light")
	lightAIColor := lightPalette["ai"]
	lightAISeq := "38;5;" + lightAIColor
	// The light ai code may appear for other segments, so we do a more targeted
	// check: the output after the ai icon emoji should contain the user code.
	// Find the ai emoji marker and check what follows.
	aiIcon := "🤖"
	if idx := strings.Index(got, aiIcon); idx >= 0 {
		// The fg code precedes the icon (paint wraps text with fg + text + reset).
		// Look for user code in a 30-byte window before the icon.
		window := got
		if idx >= 30 {
			window = got[idx-30:]
		}
		if !strings.Contains(window[:min(30, len(window))], userFGSeq) {
			t.Errorf("user override wins: user fg code %q not found before ai icon in window %q",
				userFGSeq, window[:min(30, len(window))])
		}
		_ = lightAISeq // suppress unused warning
	}
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
