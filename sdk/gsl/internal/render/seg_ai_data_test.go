package render

// seg_ai_data_test.go — spec gsl-ultra E7 (feature F4: the model name).
//
// The live repro, captured on 2026-07-11 from a real Claude Code session:
//
//	🤖 context) 🧠 42%
//
// shortenModelName returned the LAST whitespace-delimited word of the display
// name. For "Claude Opus 4.8 (1M context)" that word is the literal string
// "context)" — a parenthesis fragment rendered as the model's name.

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// utf8ValidString is a readability alias used by the rune-safety assertions.
func utf8ValidString(s string) bool { return utf8.ValidString(s) }

func TestShortenModelName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// ── the live repro (E7) ──────────────────────────────────────────────
		{"live repro: opus with parenthetical", "Claude Opus 4.8 (1M context)", "Opus 4.8"},

		// ── the alias table: family + version ────────────────────────────────
		{"sonnet", "Claude Sonnet 4.5", "Sonnet 4.5"},
		{"haiku with parenthetical", "Claude Haiku 4.5 (fast)", "Haiku 4.5"},
		{"gemini", "Gemini 2.5 Pro", "Gemini 2.5"},
		{"gpt", "OpenAI GPT 5.2 Turbo", "GPT 5.2"},
		{"case insensitive", "claude opus 4.8 (1m context)", "Opus 4.8"},
		{"family with no version", "Claude Opus (preview)", "Opus"},

		// ── short names pass through untouched ───────────────────────────────
		{"already short", "Opus 4.7", "Opus 4.7"},
		{"very short", "gpt-4", "gpt-4"},
		{"empty", "", ""},

		// ── fallback: the FIRST meaningful token, never the trailing junk ────
		{"unknown family", "Mystery Model Nine Thousand", "Mystery"},
		{"unknown family, leading punctuation", "(beta) Zephyrus Ultra", "Zephyrus"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortenModelName(tc.in)
			if got != tc.want {
				t.Errorf("shortenModelName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestShortenModelName_NeverTrailingParenthetical is the regression assertion
// stated as the defect itself: whatever else changes, the shortened name must
// never be the trailing parenthetical fragment.
func TestShortenModelName_NeverTrailingParenthetical(t *testing.T) {
	for _, in := range []string{
		"Claude Opus 4.8 (1M context)",
		"Claude Sonnet 4.5 (200k context)",
		"Some Model (experimental)",
	} {
		got := shortenModelName(in)
		if strings.Contains(got, ")") || strings.Contains(got, "(") {
			t.Errorf("shortenModelName(%q) = %q — must never be a parenthetical fragment", in, got)
		}
		if got == "context)" {
			t.Errorf("shortenModelName(%q) = %q — THE live bug", in, got)
		}
	}
}

// TestShortenModelName_RuneSafe proves the fallback prefix-cut never splits a
// multi-byte rune. The shipped code did `name[:8]`, a BYTE slice, which cuts a
// 3-byte CJK rune in half and emits replacement characters.
func TestShortenModelName_RuneSafe(t *testing.T) {
	for _, in := range []string{
		"模型名称非常长的一个中文模型",               // CJK: every rune is 3 bytes / 2 columns
		"Модель Очень Длинная Русская", // Cyrillic: 2 bytes per rune
		"🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀",                   // emoji: 4 bytes per rune
	} {
		got := shortenModelName(in)
		if !utf8ValidString(got) {
			t.Errorf("shortenModelName(%q) = %q — not valid UTF-8 (byte-sliced a rune)", in, got)
		}
		if strings.ContainsRune(got, '�') {
			t.Errorf("shortenModelName(%q) = %q — contains U+FFFD (split a rune)", in, got)
		}
		if w := term.DisplayWidth(got); w > 8 {
			t.Errorf("shortenModelName(%q) = %q — display width %d exceeds the 8-column budget", in, got, w)
		}
	}
}
