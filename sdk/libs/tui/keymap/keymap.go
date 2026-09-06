// Package keymap makes a TUI's keys data: an ordered table of bindings that
// renders its own footer and help, looks up actions from real key events, and
// dispatches to handlers. Keys are bubbletea KeyMsg.String() names.
package keymap

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Action names what a key does. Tools add their own ("toggle", "refresh").
type Action string

// The canonical actions (GUIDE.md §3).
const (
	Up             Action = "up"
	Down           Action = "down"
	PageLeft       Action = "page-left"
	PageRight      Action = "page-right"
	Top            Action = "top"
	Bottom         Action = "bottom"
	HalfUp         Action = "half-up"
	HalfDown       Action = "half-down"
	PageUp         Action = "page-up"
	PageDown       Action = "page-down"
	Search         Action = "search"
	NextMatch      Action = "next-match"
	PrevMatch      Action = "prev-match"
	ClearHighlight Action = "clear-highlight"
	Command        Action = "command"
	Help           Action = "help"
	Quit           Action = "quit"
	Select         Action = "select"
	Confirm        Action = "confirm"
	Back           Action = "back"
)

// Binding is one row of the key table.
type Binding struct {
	Action Action
	Keys   []string // KeyMsg.String() names; " " is written "space"; "gg" is the chord nav owns
	Help   string   // overlay text
	Short  string   // footer text; defaults to Help
	Group  string   // footer: bindings sharing a Group render as one item ("j/k move")
	Icon   string   // optional glyph for the overlay
	Header bool     // shown in the footer
}

// Map is an ordered key table. Order is presentation order.
type Map []Binding

// KeyName canonicalizes a key event to the name used in Binding.Keys.
func KeyName(msg tea.KeyMsg) string {
	if s := msg.String(); s != " " {
		return s
	}
	return "space"
}

// Lookup returns the action bound to the key event.
func (m Map) Lookup(msg tea.KeyMsg) (Action, bool) {
	name := KeyName(msg)
	for _, b := range m {
		for _, k := range b.Keys {
			if k == name {
				return b.Action, true
			}
		}
	}
	return "", false
}

// Keys returns the keys bound to an action (nil when unbound).
func (m Map) Keys(a Action) []string {
	for _, b := range m {
		if b.Action == a {
			return b.Keys
		}
	}
	return nil
}

// Has reports whether any binding lists the key name (chords included).
func (m Map) Has(key string) bool {
	for _, b := range m {
		for _, k := range b.Keys {
			if k == key {
				return true
			}
		}
	}
	return false
}

// Merge returns a copy with each binding replaced in place (same Action) or
// appended. The receiver is never mutated.
func (m Map) Merge(bs ...Binding) Map {
	out := append(Map(nil), m...)
	for _, nb := range bs {
		replaced := false
		for i := range out {
			if out[i].Action == nb.Action {
				out[i] = nb
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, nb)
		}
	}
	return out
}

// Without returns a copy with the actions removed.
func (m Map) Without(as ...Action) Map {
	drop := map[Action]bool{}
	for _, a := range as {
		drop[a] = true
	}
	out := make(Map, 0, len(m))
	for _, b := range m {
		if !drop[b.Action] {
			out = append(out, b)
		}
	}
	return out
}

// HelpRow is one overlay line.
type HelpRow struct {
	Icon, Keys, Help string
	Header           bool
}

// HelpRows renders every binding for the help overlay, keys joined by "/".
func (m Map) HelpRows() []HelpRow {
	rows := make([]HelpRow, len(m))
	for i, b := range m {
		rows[i] = HelpRow{Icon: b.Icon, Keys: strings.Join(b.Keys, "/"), Help: b.Help, Header: b.Header}
	}
	return rows
}

// HeaderHint renders the footer strip from Header bindings in table order.
// Bindings sharing a Group collapse into one item whose keys are each
// member's primary key ("j/k move").
func (m Map) HeaderHint(sep string) string {
	type item struct{ keys, short string }
	var items []item
	groupAt := map[string]int{}
	for _, b := range m {
		if !b.Header || len(b.Keys) == 0 {
			continue
		}
		short := b.Short
		if short == "" {
			short = b.Help
		}
		if b.Group != "" {
			if i, ok := groupAt[b.Group]; ok {
				items[i].keys += "/" + b.Keys[0]
				continue
			}
			groupAt[b.Group] = len(items)
		}
		items = append(items, item{keys: b.Keys[0], short: short})
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = it.keys + " " + it.short
	}
	return strings.Join(parts, sep)
}

// Handlers binds actions to behavior.
type Handlers map[Action]func() tea.Cmd

// Dispatch looks the key up and runs its handler. handled is false when the
// key is unbound or the action has no handler, so callers can fall through.
func Dispatch(m Map, msg tea.KeyMsg, h Handlers) (bool, tea.Cmd) {
	a, ok := m.Lookup(msg)
	if !ok {
		return false, nil
	}
	fn, ok := h[a]
	if !ok {
		return false, nil
	}
	return true, fn()
}
