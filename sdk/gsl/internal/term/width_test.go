// Package term provides terminal-width helpers and display-width measurement.
package term

import (
	"testing"
)

// ─── Columns ──────────────────────────────────────────────────────────────────

func TestColumns_SourcePrecedence(t *testing.T) {
	// When the injected source returns a value, it wins over the 80 fallback.
	cols := Columns(func() (int, bool) { return 60, true })
	if cols != 60 {
		t.Errorf("Columns: want 60 from source, got %d", cols)
	}
}

func TestColumns_FallbackWhenSourceFalse(t *testing.T) {
	// When the source returns false, Columns falls back to 80.
	// (In production the real source would be an ioctl; here we simulate no tty.)
	cols := Columns(func() (int, bool) { return 0, false })
	if cols != 80 {
		t.Errorf("Columns fallback: want 80, got %d", cols)
	}
}

func TestColumns_NilSourceFallback(t *testing.T) {
	cols := Columns(nil)
	if cols != 80 {
		t.Errorf("Columns nil source: want 80, got %d", cols)
	}
}

func TestColumns_COLUMNS_Env(t *testing.T) {
	// $COLUMNS overrides even the injected source.
	t.Setenv("COLUMNS", "50")
	cols := Columns(func() (int, bool) { return 60, true })
	if cols != 50 {
		t.Errorf("Columns $COLUMNS: want 50 (env wins), got %d", cols)
	}
}

func TestColumns_InvalidCOLUMNS_FallsThrough(t *testing.T) {
	// Malformed $COLUMNS is ignored; source wins next.
	t.Setenv("COLUMNS", "notanumber")
	cols := Columns(func() (int, bool) { return 72, true })
	if cols != 72 {
		t.Errorf("Columns invalid $COLUMNS: want 72 (source), got %d", cols)
	}
}

// ─── DisplayWidth ─────────────────────────────────────────────────────────────

func TestDisplayWidth_ASCIIOnly(t *testing.T) {
	if w := DisplayWidth("hello"); w != 5 {
		t.Errorf("DisplayWidth ASCII: want 5, got %d", w)
	}
}

func TestDisplayWidth_ANSIStripped(t *testing.T) {
	// ANSI SGR sequences must not count toward display width.
	colored := "\x1b[38;5;12mhello\x1b[0m"
	if w := DisplayWidth(colored); w != 5 {
		t.Errorf("DisplayWidth ANSI-stripped: want 5, got %d", w)
	}
}

func TestDisplayWidth_Empty(t *testing.T) {
	if w := DisplayWidth(""); w != 0 {
		t.Errorf("DisplayWidth empty: want 0, got %d", w)
	}
}

// ─── Per-icon width fixtures (source of truth = emojiStyle.Icons) ─────────────
//
// 2-col emoji: 📁🏠🌳🤖🔌⏰🌿🧠📦
// 1-col emoji: ⬆⬇✚✎✦⑂
//
// We assert the uniseg.StringWidth value, NOT physical terminal cells.
// This makes the test deterministic regardless of the terminal in use.

func TestDisplayWidth_TwoColEmoji(t *testing.T) {
	twoCols := []struct {
		name  string
		glyph string
	}{
		{"folder", "📁"},
		{"home", "🏠"},
		{"tree", "🌳"},
		{"robot", "🤖"},
		{"plug", "🔌"},
		{"clock", "⏰"},
		{"herb", "🌿"},
		{"brain", "🧠"},
		{"box", "📦"},
	}
	for _, tc := range twoCols {
		t.Run(tc.name, func(t *testing.T) {
			if w := DisplayWidth(tc.glyph); w != 2 {
				t.Errorf("DisplayWidth 2-col %s (%q): want 2, got %d", tc.name, tc.glyph, w)
			}
		})
	}
}

func TestDisplayWidth_OneColEmoji(t *testing.T) {
	oneCols := []struct {
		name  string
		glyph string
	}{
		{"ahead", "⬆"},
		{"behind", "⬇"},
		{"staged", "✚"},
		{"unstaged", "✎"},
		{"untracked", "✦"},
		{"worktree_count", "⑂"},
	}
	for _, tc := range oneCols {
		t.Run(tc.name, func(t *testing.T) {
			if w := DisplayWidth(tc.glyph); w != 1 {
				t.Errorf("DisplayWidth 1-col %s (%q): want 1, got %d", tc.name, tc.glyph, w)
			}
		})
	}
}

func TestDisplayWidth_CJK(t *testing.T) {
	// CJK unified ideographs are East-Asian wide (2 cols each).
	if w := DisplayWidth("中"); w != 2 {
		t.Errorf("DisplayWidth CJK '中': want 2, got %d", w)
	}
	if w := DisplayWidth("中文"); w != 4 {
		t.Errorf("DisplayWidth CJK '中文': want 4, got %d", w)
	}
}

func TestDisplayWidth_MixedANSIAndEmoji(t *testing.T) {
	// ANSI-colored 2-col emoji: strip the escape, measure the glyph.
	s := "\x1b[38;5;6m🤖\x1b[0m text"
	// 🤖 = 2, " text" = 5, total = 7
	if w := DisplayWidth(s); w != 7 {
		t.Errorf("DisplayWidth mixed: want 7, got %d", w)
	}
}

// TestStripANSI_OSC8 covers the hyperlink sequences the join layer wraps around
// linkable segments. These are OSC, not CSI: before OSC support, every byte of
// the embedded URL counted as display width, which would make the fit loop shed
// segments that actually fit.
func TestStripANSI_OSC8(t *testing.T) {
	const url = "https://github.com/o/r/pull/247"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ST-terminated hyperlink",
			in:   "\x1b]8;;" + url + "\x1b\\PR#247\x1b]8;;\x1b\\",
			want: "PR#247",
		},
		{
			name: "BEL-terminated hyperlink",
			in:   "\x1b]8;;" + url + "\aPR#247\x1b]8;;\a",
			want: "PR#247",
		},
		{
			name: "hyperlink wrapping SGR-tinted text",
			in:   "\x1b]8;;" + url + "\x1b\\\x1b[38;5;2mPR#247\x1b[0m\x1b]8;;\x1b\\",
			want: "PR#247",
		},
		{
			name: "unterminated OSC consumes to end of string",
			in:   "abc\x1b]8;;" + url,
			want: "abc",
		},
		{
			name: "text with no escapes is returned unchanged",
			in:   "PR#247",
			want: "PR#247",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripANSI(tc.in); got != tc.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestDisplayWidth_OSC8IsZeroWidth is the invariant the fit loop depends on:
// wrapping text in a hyperlink must not change its measured width, however long
// the URL is.
func TestDisplayWidth_OSC8IsZeroWidth(t *testing.T) {
	plain := "PR#247"
	linked := "\x1b]8;;https://github.com/some-org/some-really-long-repo-name/pull/247\x1b\\PR#247\x1b]8;;\x1b\\"
	if got, want := DisplayWidth(linked), DisplayWidth(plain); got != want {
		t.Errorf("DisplayWidth(linked) = %d, want %d (same as unlinked)", got, want)
	}
}
