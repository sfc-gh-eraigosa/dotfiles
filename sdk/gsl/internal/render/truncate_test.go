package render

// truncate_test.go — unit tests for the truncation tier (spec E3's guarantee).

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		cols int
		want string
	}{
		{"fits exactly", "hello", 5, "hello"},
		{"fits with room", "hello", 20, "hello"},
		{"cut ascii", "hello world", 8, "hello w…"},
		{"one column", "hello", 1, "…"},
		{"zero columns", "hello", 0, ""},
		{"negative columns", "hello", -3, ""},
		{"empty input", "", 10, ""},
		// A CJK ideograph is ONE grapheme occupying TWO columns. A rune-count cut
		// would fit 4 of them in a 5-column budget; a column-count cut fits 2 plus
		// the ellipsis.
		{"cjk is two columns per rune", "文档目录内容", 5, "文档…"},
		{"cjk exact", "文档", 4, "文档"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateText(tc.in, tc.cols)
			if got != tc.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tc.in, tc.cols, got, tc.want)
			}
			if w := term.DisplayWidth(got); w > tc.cols && tc.cols > 0 {
				t.Errorf("truncateText(%q, %d) = %q has width %d > %d", tc.in, tc.cols, got, w, tc.cols)
			}
		})
	}
}

// TestTruncateText_NeverExceedsWidth sweeps every budget over inputs whose
// grapheme clusters are hostile to naive cutting.
func TestTruncateText_NeverExceedsWidth(t *testing.T) {
	inputs := []string{
		"plain-ascii-repository-name",
		"文档目录内容非常长的中文路径",         // 2 cols/rune, 3 bytes/rune
		"feat/🚀-launch-👩‍🚀-crew", // ZWJ cluster: 1 grapheme, many runes
		"Ünïcödé-áccèntéd-repo",  // combining-capable Latin
		"🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀",           // all wide emoji
	}
	for _, in := range inputs {
		for cols := 1; cols <= 40; cols++ {
			got := truncateText(in, cols)
			if w := term.DisplayWidth(got); w > cols {
				t.Fatalf("truncateText(%q, %d) = %q → width %d > %d", in, cols, got, w, cols)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncateText(%q, %d) = %q → invalid UTF-8", in, cols, got)
			}
			if strings.ContainsRune(got, '�') {
				t.Fatalf("truncateText(%q, %d) = %q → contains U+FFFD (split a rune)", in, cols, got)
			}
			if got == "" {
				t.Fatalf("truncateText(%q, %d) = \"\" → must never delete the content entirely", in, cols)
			}
		}
	}
}

// TestTruncateText_DoesNotSplitZWJCluster proves the ZWJ astronaut survives or
// disappears WHOLE — it is never cut into its constituent emoji.
func TestTruncateText_DoesNotSplitZWJCluster(t *testing.T) {
	const astronaut = "👩‍🚀" // U+1F469 U+200D U+1F680 — one grapheme cluster
	in := "ab" + astronaut + "cd"

	for cols := 1; cols <= term.DisplayWidth(in); cols++ {
		got := truncateText(in, cols)
		// The cluster's PARTS must never appear without the whole cluster.
		hasWhole := strings.Contains(got, astronaut)
		hasWoman := strings.Contains(got, "\U0001F469")
		if hasWoman && !hasWhole {
			t.Fatalf("cols=%d: truncateText split the ZWJ cluster: %q", cols, got)
		}
		if strings.HasSuffix(got, "\u200d") {
			t.Fatalf("cols=%d: truncateText left a dangling ZWJ: %q", cols, got)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	st := emojiStyleFixture() // Separator "thin", Fill false → 3-column separator

	blocks := []segmentBlock{
		{text: "dotfiles", colorKey: "dirgit"},
		{text: "gsl-ultra", colorKey: "repo_root"},
	}

	t.Run("already fits: returned untouched", func(t *testing.T) {
		got := truncateToWidth(blocks, st, 100)
		if len(got) != 2 || got[0].text != "dotfiles" || got[1].text != "gsl-ultra" {
			t.Errorf("want blocks unchanged, got %+v", got)
		}
	})

	t.Run("narrow: joined result fits", func(t *testing.T) {
		for cols := 1; cols <= 40; cols++ {
			got := truncateToWidth(blocks, st, cols)
			if len(got) == 0 {
				continue // decoration alone cannot fit; caller falls back
			}
			if w := term.DisplayWidth(join(st, got)); w > cols {
				t.Fatalf("cols=%d: joined truncation width %d > %d (%q)", cols, w, cols, join(st, got))
			}
		}
	})

	t.Run("empty input and non-positive cols", func(t *testing.T) {
		if got := truncateToWidth(nil, st, 20); got != nil {
			t.Errorf("nil blocks: want nil, got %+v", got)
		}
		if got := truncateToWidth(blocks, st, 0); got != nil {
			t.Errorf("cols=0: want nil, got %+v", got)
		}
	})

	t.Run("strips embedded ANSI before cutting", func(t *testing.T) {
		// prBadge embeds an SGR tint inside the segment text. A byte-cut landing
		// inside "\x1b[38;5;2m" would spray "8;5;2m" onto the line.
		painted := []segmentBlock{{text: "repo \x1b[38;5;2mPR#157\x1b[0m", colorKey: "repo_root"}}
		got := truncateToWidth(painted, st, 8)
		if len(got) != 1 {
			t.Fatalf("want 1 block, got %d", len(got))
		}
		if strings.Contains(got[0].text, "\x1b") {
			t.Errorf("truncated text still carries an escape byte: %q", got[0].text)
		}
		if strings.Contains(got[0].text, "38;5;2") {
			t.Errorf("truncated text leaked an escape body as literal text: %q", got[0].text)
		}
	})
}

func TestJoinOverhead(t *testing.T) {
	// Overhead must depend only on the block COUNT, not on the text — otherwise
	// the budget computed before truncation is wrong after it, and the width
	// invariant becomes an accident rather than a guarantee.
	for name, st := range fitStyles() {
		t.Run(name, func(t *testing.T) {
			short := []segmentBlock{{text: "a", colorKey: "dirgit"}, {text: "b", colorKey: "repo_root"}}
			long := []segmentBlock{{text: "aaaaaaaaaaaa", colorKey: "dirgit"}, {text: "bbbbbbbb", colorKey: "repo_root"}}
			if o1, o2 := joinOverhead(st, short), joinOverhead(st, long); o1 != o2 {
				t.Errorf("overhead depends on text: %d (short) != %d (long)", o1, o2)
			}
			if got := joinOverhead(st, nil); got != 0 {
				t.Errorf("joinOverhead(nil) = %d, want 0", got)
			}
		})
	}
}
