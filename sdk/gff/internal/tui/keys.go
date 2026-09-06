package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
)

// actUnset is gff's own action layered on the sdk vim map (libs/tui/GUIDE.md §3).
const actUnset keymap.Action = "unset"

// gffKeys is the single key table: footer, help overlay, `gff tui --help`,
// and the README table all render from (or are pinned to) it.
var gffKeys = keymap.Vim.Merge(
	// gff's lateral axis is the category breadcrumb, so the sdk page bindings
	// say so in the overlay (GUIDE.md §3); the footer keeps the short "page".
	keymap.Binding{Action: keymap.PageLeft, Keys: []string{"h", "left"}, Help: "previous category page", Short: "page", Group: "page", Header: true},
	keymap.Binding{Action: keymap.PageRight, Keys: []string{"l", "right"}, Help: "next category page", Short: "page", Group: "page", Header: true},
	keymap.Binding{Action: keymap.Select, Keys: []string{"space"}, Help: "toggle a bool / pick choice options (same writer as `gff set`)", Short: "toggle", Header: true},
	keymap.Binding{Action: keymap.Confirm, Keys: []string{"enter"}, Help: "expand an area / open feature details (attributes + layers)", Short: "open", Header: true},
	keymap.Binding{Action: actUnset, Keys: []string{"u"}, Help: "clear the user override for the row (same as `gff unset`)", Short: "clear", Header: true},
)

// listHint is the normal-mode footer.
func listHint() string { return gffKeys.HeaderHint("  ") }

// palette adapts internal/style to overlay.Palette; NO_COLOR → plain text.
type palette struct{ pal style.Colors }

func newPalette() overlay.Palette {
	if noColor() {
		return overlay.Plain{}
	}
	return palette{pal: style.Active()}
}

func (p palette) Dim(s string) string { return lipgloss.NewStyle().Foreground(p.pal.Grey).Render(s) }
func (p palette) Bold(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(p.pal.Purple).Render(s)
}
func (p palette) Accent(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(p.pal.Text).Render(s)
}
func (p palette) Err(s string) string { return lipgloss.NewStyle().Foreground(p.pal.Red).Render(s) }
