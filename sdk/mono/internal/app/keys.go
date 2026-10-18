package app

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

// mono's own actions, on top of the shared sdk keymap.
const (
	ActRestart    keymap.Action = "restart"
	ActSendPrefix keymap.Action = "send-prefix"
)

// Keys is mono's key table: keymap.Vim trimmed to what a stack of panes
// needs, plus restart and the literal prefix. Every key here is pressed after
// the prefix. The help overlay, the title-bar hint, `mono --help` and the
// README all render from this one table.
func Keys(prefix string) keymap.Map {
	return keymap.Vim.Without(
		keymap.PageLeft, keymap.PageRight, keymap.Top, keymap.Bottom,
		keymap.HalfDown, keymap.HalfUp, keymap.PageDown, keymap.PageUp,
		keymap.Search, keymap.NextMatch, keymap.PrevMatch, keymap.ClearHighlight,
		keymap.Select, keymap.Confirm,
	).Merge(
		keymap.Binding{Action: keymap.Down, Keys: []string{"j", "down"}, Help: "focus the next pane", Short: "pane", Group: "pane", Header: true},
		keymap.Binding{Action: keymap.Up, Keys: []string{"k", "up"}, Help: "focus the previous pane", Short: "pane", Group: "pane", Header: true},
		keymap.Binding{Action: keymap.Command, Keys: []string{":"}, Help: "command line (see Commands below)", Short: "command", Header: true},
		keymap.Binding{Action: keymap.Help, Keys: []string{"?", "f1"}, Help: "this help", Short: "help", Header: true},
		keymap.Binding{Action: ActRestart, Keys: []string{"r"}, Help: "restart the focused pane's program", Short: "restart", Header: true},
		keymap.Binding{Action: keymap.Quit, Keys: []string{"q", "ctrl+c"}, Help: "quit mono (stops every pane)", Short: "quit", Header: true},
		keymap.Binding{Action: keymap.Back, Keys: []string{"esc"}, Help: "cancel the prefix"},
		keymap.Binding{Action: ActSendPrefix, Keys: []string{prefix}, Help: "send " + prefix + " itself to the program"},
	)
}
