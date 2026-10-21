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
	got := paint(st, "repo_root", "X", nil)
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
	got := paint(st, "repo_worktree", "X", nil)
	if !strings.Contains(got, "\x1b[38;5;5m") {
		t.Errorf("paint no-fill: missing magenta fg in %q", got)
	}
	if strings.Contains(got, "48;5;") {
		t.Errorf("paint no-fill: should not emit a background, got %q", got)
	}
}

func TestPaint_NoColor_PlainText(t *testing.T) {
	st := style.Style{Fill: false} // no theme at all
	got := paint(st, "repo_root", "plain", nil)
	if got != "plain" {
		t.Errorf("paint with no theme should be plain, got %q", got)
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

// A filled block with the default fg gets whichever of black or white reads
// better on it: white on a dark block (and on named colors, which the
// terminal themes), black on a light one — the shared dark palette's ok

// fillText also reads 38;5;N and truecolor fills, the forms a user config may

// A filled block with the default fg gets readable text: white on a
// terminal-themed 0–15 block, black or white by contrast on anything else —
// in every colour form a config may hold.
func TestPaint_FillPicksReadableText(t *testing.T) {
	for bg, wantFG := range map[string]string{
		"blue": "\x1b[38;5;7m", "26": "\x1b[38;5;15m", "124": "\x1b[38;5;15m",
		"42": "\x1b[38;5;16m", "214": "\x1b[38;5;16m", "38;5;214": "\x1b[38;5;16m",
		"#fafad2": "\x1b[38;5;16m", "38;2;20;20;60": "\x1b[38;5;15m",
	} {
		st := style.Style{Fill: true, Theme: map[string]string{"repo_root": bg, "fg": "default"}}
		if got := paint(st, "repo_root", "X", nil); !strings.Contains(got, wantFG) {
			t.Errorf("bg %s: %q lacks fg %q", bg, got, wantFG)
		}
	}
}

// E14: whatever the theme holds, every powerline block emits a background and
// a foreground, and the trailing chevron is never a bare (unpainted) wedge.
func TestJoinPowerline_NoUnpaintedBlock(t *testing.T) {
	for _, v := range []string{"default", "#00ff00", "nope", ""} {
		st := powerlineStyleFixture()
		st.Theme = map[string]string{"repo_root": v, "dirgit": v, "fg": "default"}
		out := join(st, []segmentBlock{{text: "a", colorKey: "dirgit"}, {text: "b", colorKey: "repo_root"}, {text: "c", colorKey: "no-such-key"}})
		blocks := strings.Split(out, ">")
		for i, b := range blocks[:len(blocks)-1] {
			if !strings.Contains(b, "\x1b[48;") || !strings.Contains(b, "\x1b[38;") {
				t.Errorf("theme %q block %d unpainted: %q", v, i, b)
			}
		}
		if strings.Contains(out, ansiReset+">") {
			t.Errorf("theme %q: a stray unpainted chevron in %q", v, out)
		}
	}
}

// E16: under NO_COLOR (ModeNever) the line carries no escape at all — no
// colour, no hyperlink — on every path.
func TestJoin_ModeNeverHasNoEscape(t *testing.T) {
	blocks := []segmentBlock{{text: "main PR#21", colorKey: "repo_root",
		links: []LinkSpan{{Start: 5, End: 10, URL: wantPRURL, Role: "ok"}}}}
	for _, st := range []style.Style{asciiStyle(), powerlineStyleFixture(), joinEmojiStyle()} {
		st.Mode = style.ModeNever
		if out := join(st, blocks); strings.ContainsRune(out, 0x1b) || !strings.Contains(out, "main PR#21") {
			t.Errorf("%s: %q", st.Separator, out)
		}
	}
}

// Colour downsamples to the terminal: a hex block on a 16-colour terminal
// uses the basic codes, on truecolor its exact value.
func TestJoin_ModeDownsamples(t *testing.T) {
	st := style.Style{Fill: true, Theme: map[string]string{"repo_root": "#00ff00"}}
	st.Mode = style.Mode16
	if got := paint(st, "repo_root", "x", nil); !strings.Contains(got, "\x1b[102m") {
		t.Errorf("16: %q", got)
	}
	st.Mode = style.ModeTruecolor
	if got := paint(st, "repo_root", "x", nil); !strings.Contains(got, "\x1b[48;2;0;255;0m") {
		t.Errorf("truecolor: %q", got)
	}
}
