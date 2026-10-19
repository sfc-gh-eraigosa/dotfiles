package app

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

// mono's own actions, on top of the shared sdk keymap.
const (
	ActRestart    keymap.Action = "restart"
	ActSendPrefix keymap.Action = "send-prefix"
	ActZoom       keymap.Action = "zoom"
	ActNav        keymap.Action = "nav"
)

// Keys is mono's key table: keymap.Vim trimmed to what a stack of panes
// needs, plus restart, zoom, nav mode and the literal prefix. Every key here
// is pressed after the prefix; nav mode (NavKeys) takes the same moves
// without it. The help overlay, the title-bar hint, `mono --help` and the
// README all render from this one table. nav is the nav-mode key ("space"
// when omitted).
func Keys(prefix string, nav ...string) keymap.Map {
	navKey := "space"
	if len(nav) > 0 && nav[0] != "" {
		navKey = nav[0]
	}
	return keymap.Vim.Without(
		keymap.Top, keymap.Bottom,
		keymap.HalfDown, keymap.HalfUp, keymap.PageDown, keymap.PageUp,
		keymap.Search, keymap.NextMatch, keymap.PrevMatch, keymap.ClearHighlight,
		keymap.Select, keymap.Confirm,
	).Merge(
		keymap.Binding{Action: keymap.Down, Keys: []string{"j", "down"}, Help: "focus the next pane", Short: "pane", Group: "pane", Header: true},
		keymap.Binding{Action: keymap.Up, Keys: []string{"k", "up"}, Help: "focus the previous pane", Short: "pane", Group: "pane", Header: true},
		// The shared table's "previous/next page / pane", re-labelled: in mono
		// h/l move between the tools of a deck pane.
		keymap.Binding{Action: keymap.PageLeft, Keys: []string{"h", "left"}, Help: "previous tool in a deck pane", Short: "tool", Group: "tool"},
		keymap.Binding{Action: keymap.PageRight, Keys: []string{"l", "right"}, Help: "next tool in a deck pane", Short: "tool", Group: "tool"},
		keymap.Binding{Action: ActZoom, Keys: []string{"z"}, Help: "zoom the pane over the middle", Short: "zoom"},
		keymap.Binding{Action: ActNav, Keys: []string{navKey}, Help: "nav mode: hjkl and z, no prefix", Short: "nav"},
		keymap.Binding{Action: keymap.Command, Keys: []string{":"}, Help: "command line (see Commands below)", Short: "command", Header: true},
		keymap.Binding{Action: keymap.Help, Keys: []string{"?", "f1"}, Help: "this help", Short: "help", Header: true},
		keymap.Binding{Action: ActRestart, Keys: []string{"r"}, Help: "restart the focused pane's program", Short: "restart", Header: true},
		keymap.Binding{Action: keymap.Quit, Keys: []string{"q", "ctrl+c"}, Help: "quit mono (stops every pane)", Short: "quit", Header: true},
		keymap.Binding{Action: keymap.Back, Keys: []string{"esc"}, Help: "cancel the prefix"},
		keymap.Binding{Action: ActSendPrefix, Keys: []string{prefix}, Help: "send " + prefix + " itself to the program"},
	)
}

// NavKeys is what nav mode understands, with no prefix: the same moves as
// the prefix table, plus esc to leave. Any other key leaves nav mode and is
// not passed to the program (spec F9).
func NavKeys() keymap.Map {
	return keymap.Map{
		{Action: keymap.Down, Keys: []string{"j", "down"}, Help: "next view of the deck, then the next pane"},
		{Action: keymap.Up, Keys: []string{"k", "up"}, Help: "previous view of the deck, then the previous pane"},
		{Action: keymap.PageLeft, Keys: []string{"h", "left"}, Help: "previous tool in a deck pane"},
		{Action: keymap.PageRight, Keys: []string{"l", "right"}, Help: "next tool in a deck pane"},
		{Action: ActZoom, Keys: []string{"z"}, Help: "zoom the focused pane over the middle"},
		{Action: keymap.Back, Keys: []string{"esc"}, Help: "leave nav mode"},
	}
}
