package render

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

func TestSeparator(t *testing.T) {
	cases := []struct {
		name string
		st   style.Style
		want string
	}{
		{
			name: "powerline uses sep_right glyph padded",
			st:   style.Style{Separator: "powerline", Icons: map[string]string{"sep_right": ">"}},
			want: " > ",
		},
		{
			name: "thin uses sep_right_thin glyph padded",
			st:   style.Style{Separator: "thin", Icons: map[string]string{"sep_right_thin": "|"}},
			want: " | ",
		},
		{
			name: "space yields single space",
			st:   style.Style{Separator: "space"},
			want: " ",
		},
		{
			name: "unknown separator yields single space",
			st:   style.Style{Separator: "weird"},
			want: " ",
		},
		{
			name: "powerline with missing glyph falls back to space",
			st:   style.Style{Separator: "powerline"},
			want: " ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := separator(tc.st); got != tc.want {
				t.Errorf("separator = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPaint_Fill(t *testing.T) {
	st := style.Style{
		Fill: true,
		Theme: map[string]string{
			"repo_root": "blue",
			"fg":        "default",
		},
	}
	got := paint(st, "repo_root", "X")
	// Expect a background blue (48;5;4), a white fg fallback (38;5;7), the text,
	// and a reset.
	if !strings.Contains(got, "\x1b[48;5;4m") {
		t.Errorf("paint fill: missing bg blue in %q", got)
	}
	if !strings.Contains(got, "\x1b[38;5;7m") {
		t.Errorf("paint fill: missing white fg fallback in %q", got)
	}
	if !strings.HasSuffix(got, ansiReset) {
		t.Errorf("paint fill: missing reset suffix in %q", got)
	}
	if !strings.Contains(got, "X") {
		t.Errorf("paint fill: missing text in %q", got)
	}
}

func TestPaint_NoFill_TintsForeground(t *testing.T) {
	st := style.Style{
		Fill:  false,
		Theme: map[string]string{"repo_worktree": "magenta"},
	}
	got := paint(st, "repo_worktree", "X")
	if !strings.Contains(got, "\x1b[38;5;5m") {
		t.Errorf("paint no-fill: missing magenta fg in %q", got)
	}
	if strings.Contains(got, "48;5;") {
		t.Errorf("paint no-fill: should not emit a background, got %q", got)
	}
}

func TestPaint_NoColor_PlainText(t *testing.T) {
	st := style.Style{Fill: false} // no theme at all
	got := paint(st, "repo_root", "plain")
	if got != "plain" {
		t.Errorf("paint with no theme should be plain, got %q", got)
	}
}

func TestColorCode(t *testing.T) {
	cases := []struct {
		value string
		layer string
		want  string
	}{
		{"blue", "38", "38;5;4"},
		{"magenta", "48", "48;5;5"},
		{"12", "38", "38;5;12"},
		{"38;5;201", "38", "38;5;201"}, // raw fragment passes through
		{"default", "38", ""},
		{"", "48", ""},
		{"notacolor", "38", ""},
	}
	for _, tc := range cases {
		if got := colorCode(tc.value, tc.layer); got != tc.want {
			t.Errorf("colorCode(%q,%q) = %q, want %q", tc.value, tc.layer, got, tc.want)
		}
	}
}

func TestCountBadge(t *testing.T) {
	st := style.Style{Icons: map[string]string{"staged": "+"}}
	if got := countBadge(st, "staged", 3); got != "+3" {
		t.Errorf("countBadge with glyph = %q, want +3", got)
	}
	// No glyph → number only.
	if got := countBadge(style.Style{}, "staged", 3); got != "3" {
		t.Errorf("countBadge without glyph = %q, want 3", got)
	}
}

// ── join (new segmentBlock API) tests ─────────────────────────────────────────

// joinPowerlineStyle returns a minimal powerline style for join tests.
// We use a ">" ASCII placeholder for sep_right to keep escape sequences legible
// in test output.
func joinPowerlineStyle() style.Style {
	return style.Style{
		Separator: "powerline",
		Fill:      true,
		Icons:     map[string]string{"sep_right": ">"},
		Theme: map[string]string{
			"fg":    "white",
			"green": "green",
			"blue":  "blue",
			"cyan":  "cyan",
		},
	}
}

// joinEmojiStyle returns a thin/no-fill style for emoji join tests.
func joinEmojiStyle() style.Style {
	return style.Style{
		Separator: "thin",
		Fill:      false,
		Icons:     map[string]string{"sep_right_thin": "·"},
		Theme: map[string]string{
			"green": "green",
			"blue":  "blue",
		},
	}
}

// TestJoinBlocks_ZeroBlocks verifies that zero blocks yields "".
func TestJoinBlocks_ZeroBlocks(t *testing.T) {
	st := joinPowerlineStyle()
	got := join(st, []segmentBlock{})
	if got != "" {
		t.Errorf("join zero blocks: want \"\", got %q", got)
	}
}

// TestJoinBlocks_SingleBlock_PowerlineFill checks that a single block with
// Fill:true emits the fill paint (bg+fg+text+reset) and a trailing chevron
// that fades fg=last color to terminal bg (no interior chevron).
func TestJoinBlocks_SingleBlock_PowerlineFill(t *testing.T) {
	st := joinPowerlineStyle()
	blocks := []segmentBlock{{text: "hello", colorKey: "green"}}
	got := join(st, blocks)

	// Must contain the painted text.
	if !strings.Contains(got, "hello") {
		t.Errorf("single block: missing text in %q", got)
	}
	// Must contain bg green (48;5;2).
	if !strings.Contains(got, "48;5;2") {
		t.Errorf("single block: missing bg green in %q", got)
	}
	// Must contain the sep glyph ">".
	if !strings.Contains(got, ">") {
		t.Errorf("single block: missing trailing chevron glyph in %q", got)
	}
	// Trailing chevron: fg=last color (green=38;5;2), no bg set.
	if !strings.Contains(got, "38;5;2") {
		t.Errorf("single block: trailing chevron should have fg=green (38;5;2), got %q", got)
	}
	// No interior chevron (only one block, no bg=next).
	// The trailing chevron must NOT carry a 48;5;N bg from a "next" block
	// (there is no next). This is checked by verifying only one bg code appears.
	bgCount := strings.Count(got, "48;5;")
	if bgCount > 1 {
		t.Errorf("single block: expected ≤1 bg code (block itself), got %d in %q", bgCount, got)
	}
}

// TestJoinBlocks_TwoBlocks_PowerlineInteriorBridge checks that between two
// powerline blocks the chevron carries bg=next,fg=prev.
func TestJoinBlocks_TwoBlocks_PowerlineInteriorBridge(t *testing.T) {
	st := joinPowerlineStyle()
	blocks := []segmentBlock{
		{text: "A", colorKey: "green"}, // green = index 2
		{text: "B", colorKey: "blue"},  // blue  = index 4
	}
	got := join(st, blocks)

	// Both texts present.
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Errorf("two blocks: missing text in %q", got)
	}

	// Interior chevron: bg=next (blue=48;5;4), fg=prev (green=38;5;2).
	// These must appear in that order before the trailing chevron.
	idxBgNext := strings.Index(got, "48;5;4")
	idxFgPrev := strings.Index(got, "38;5;2")
	if idxBgNext < 0 {
		t.Errorf("interior chevron: bg=next (blue, 48;5;4) not found in %q", got)
	}
	if idxFgPrev < 0 {
		t.Errorf("interior chevron: fg=prev (green, 38;5;2) not found in %q", got)
	}

	// Trailing chevron after the last block must fade fg=blue (38;5;4).
	// The last occurrence of "38;5;4" should be after "B".
	idxB := strings.LastIndex(got, "B")
	lastFgBlue := strings.LastIndex(got, "38;5;4")
	if lastFgBlue < idxB {
		t.Errorf("trailing chevron: fg=last (blue, 38;5;4) should appear after B; got %q", got)
	}
}

// TestJoinBlocks_ThreeBlocks_PowerlineBridges checks a three-block powerline
// render has two interior chevrons and one trailing chevron.
func TestJoinBlocks_ThreeBlocks_PowerlineBridges(t *testing.T) {
	st := joinPowerlineStyle()
	blocks := []segmentBlock{
		{text: "A", colorKey: "green"}, // green=2
		{text: "B", colorKey: "blue"},  // blue=4
		{text: "C", colorKey: "cyan"},  // cyan=6
	}
	got := join(st, blocks)

	for _, m := range []string{"A", "B", "C"} {
		if !strings.Contains(got, m) {
			t.Errorf("three blocks: missing %q in %q", m, got)
		}
	}
	// There must be a glyph for each of the two interior bridges + one trailing.
	chevronCount := strings.Count(got, ">")
	if chevronCount != 3 {
		t.Errorf("three blocks: want 3 chevrons (2 interior + 1 trailing), got %d in %q", chevronCount, got)
	}
}

// TestJoinBlocks_Emoji_Unchanged verifies that the emoji/thin path produces
// space-padded glyph separators and NO bg colors (Fill:false).
func TestJoinBlocks_Emoji_Unchanged(t *testing.T) {
	st := joinEmojiStyle()
	blocks := []segmentBlock{
		{text: "A", colorKey: "green"},
		{text: "B", colorKey: "blue"},
	}
	got := join(st, blocks)

	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Errorf("emoji join: missing text in %q", got)
	}
	// Must contain the thin separator "·" padded.
	if !strings.Contains(got, " · ") {
		t.Errorf("emoji join: missing thin separator in %q", got)
	}
	// Must NOT contain any background escape (48;5;N) — Fill:false, no bridges.
	if strings.Contains(got, "48;5;") {
		t.Errorf("emoji join: should not emit background colors, got %q", got)
	}
}
