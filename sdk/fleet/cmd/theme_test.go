package cmd

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestNewTheme_PlainASCIIBordersOnRequest pins a fix for garbled panel
// borders reported over Terminus on iPadOS: lipgloss's default RoundedBorder
// draws its top/bottom edge with "─" (U+2500), and some mobile SSH clients'
// monospace fonts substitute a wider fallback glyph for codepoints outside
// Basic Latin — with the row's width computed assuming the narrow original,
// every line below the border then reads as shifted. FLEET_ASCII_BORDERS=1
// opts into lipgloss's pure-ASCII border (`-`/`|`/`+`) instead, which every
// terminal renders correctly.
func TestNewTheme_PlainASCIIBordersOnRequest(t *testing.T) {
	t.Setenv("FLEET_ASCII_BORDERS", "1")
	th := newTheme()

	for name, b := range map[string]lipgloss.Border{
		"panel":  th.panel.GetBorderStyle(),
		"dialog": th.dialog.GetBorderStyle(),
	} {
		if b.Top != "-" {
			t.Errorf("%s border top = %q; want ASCII \"-\"", name, b.Top)
		}
		if b.Left != "|" {
			t.Errorf("%s border left = %q; want ASCII \"|\"", name, b.Left)
		}
	}
}

// TestNewTheme_RoundedBordersByDefault pins the unset-env default so a
// regression in the other direction (ASCII always on) fails just as loudly.
func TestNewTheme_RoundedBordersByDefault(t *testing.T) {
	t.Setenv("FLEET_ASCII_BORDERS", "")
	th := newTheme()

	b := th.panel.GetBorderStyle()
	if b.Top != "─" {
		t.Errorf("panel border top = %q; want the rounded-border default \"─\"", b.Top)
	}
	if b.TopLeft != "╭" {
		t.Errorf("panel border top-left = %q; want the rounded-border default \"╭\"", b.TopLeft)
	}
}
