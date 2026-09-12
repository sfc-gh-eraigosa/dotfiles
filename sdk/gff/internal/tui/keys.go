package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
)

// gff's own actions layered on the sdk vim map (libs/tui/GUIDE.md §3).
const (
	actUnset      keymap.Action = "unset"
	actSourceNext keymap.Action = "source-next"
	actSourcePrev keymap.Action = "source-prev"
)

// gffKeys is the single key table: footer, help overlay, `gff tui --help`,
// and the README table all render from (or are pinned to) it.
var gffKeys = insertAfter(keymap.Vim.Merge(
	// gff's lateral axis is the category breadcrumb, so the sdk page bindings
	// say so in the overlay (GUIDE.md §3); the footer keeps the short "page".
	keymap.Binding{Action: keymap.PageLeft, Keys: []string{"h", "left"}, Help: "previous category page (within the source)", Short: "page", Group: "page", Header: true},
	keymap.Binding{Action: keymap.PageRight, Keys: []string{"l", "right"}, Help: "next category page (within the source)", Short: "page", Group: "page", Header: true},
	keymap.Binding{Action: keymap.Select, Keys: []string{"space"}, Help: "toggle a bool / pick choice options (same writer as `gff set`)", Short: "toggle", Header: true},
	keymap.Binding{Action: keymap.Confirm, Keys: []string{"enter"}, Help: "expand an area / open feature details (attributes + layers)", Short: "open", Header: true},
	keymap.Binding{Action: actUnset, Keys: []string{"u"}, Help: "clear the user override for the row (same as `gff unset`)", Short: "clear", Header: true},
	// gff answered to Q before it adopted the sdk map, and 'u'/'U' both still
	// clear an override — the uppercase alias stays, declared here rather than
	// special-cased in the handler.
	keymap.Binding{Action: keymap.Quit, Keys: []string{"q", "Q", "ctrl+c"}, Help: "quit", Short: "quit", Header: true},
), keymap.PageRight,
	// The source axis sits beside h/l in the footer: it is how the breadcrumb
	// reaches a second source's categories, so it must not be the entry the
	// terminal width cuts off. Only Tab is advertised there; the overlay lists both.
	keymap.Binding{Action: actSourceNext, Keys: []string{"tab"}, Help: "next source (namespace) — its categories on h/l", Short: "source", Group: "source", Header: true},
	keymap.Binding{Action: actSourcePrev, Keys: []string{"shift+tab"}, Help: "previous source (namespace)", Short: "source", Group: "source"},
)

// insertAfter returns a copy of km with bs placed right after the binding for
// action a (appended when a is unbound). Table order is presentation order.
func insertAfter(km keymap.Map, a keymap.Action, bs ...keymap.Binding) keymap.Map {
	out := make(keymap.Map, 0, len(km)+len(bs))
	placed := false
	for _, b := range km {
		out = append(out, b)
		if b.Action == a && !placed {
			out = append(out, bs...)
			placed = true
		}
	}
	if !placed {
		out = append(out, bs...)
	}
	return out
}

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
