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

func TestJoin(t *testing.T) {
	st := style.Style{Separator: "space"}
	if got := join(st, []string{"a", "b", "c"}); got != "a b c" {
		t.Errorf("join = %q, want 'a b c'", got)
	}
}
