package keymap

// Vim is the canonical sdk keymap (GUIDE.md §3). Tools extend it with Merge
// and trim it with Without; they do not rebind these keys.
var Vim = Map{
	{Action: Down, Keys: []string{"j", "down"}, Help: "move down", Short: "move", Group: "move", Header: true},
	{Action: Up, Keys: []string{"k", "up"}, Help: "move up", Short: "move", Group: "move", Header: true},
	{Action: PageLeft, Keys: []string{"h", "left"}, Help: "previous page / pane", Short: "page", Group: "page", Header: true},
	{Action: PageRight, Keys: []string{"l", "right"}, Help: "next page / pane", Short: "page", Group: "page", Header: true},
	{Action: Top, Keys: []string{"gg"}, Help: "first row"},
	{Action: Bottom, Keys: []string{"G"}, Help: "last row"},
	{Action: HalfDown, Keys: []string{"ctrl+d"}, Help: "half page down"},
	{Action: HalfUp, Keys: []string{"ctrl+u"}, Help: "half page up"},
	{Action: PageDown, Keys: []string{"ctrl+f", "pgdown"}, Help: "page down"},
	{Action: PageUp, Keys: []string{"ctrl+b", "pgup"}, Help: "page up"},
	{Action: Search, Keys: []string{"/"}, Help: "regex search (smartcase; enter commit, esc cancel)", Short: "search", Header: true},
	{Action: NextMatch, Keys: []string{"n"}, Help: "next match", Short: "match", Group: "match", Header: true},
	{Action: PrevMatch, Keys: []string{"N"}, Help: "previous match", Short: "match", Group: "match", Header: true},
	{Action: ClearHighlight, Keys: []string{"esc"}, Help: "clear search highlights (pattern kept for n/N)"},
	{Action: Command, Keys: []string{":"}, Help: "command line", Short: "command", Header: true},
	{Action: Help, Keys: []string{"?", "f1"}, Help: "this help", Short: "help", Header: true},
	{Action: Quit, Keys: []string{"q", "ctrl+c"}, Help: "quit", Header: true},
	{Action: Select, Keys: []string{"space"}, Help: "select / toggle the row"},
	{Action: Confirm, Keys: []string{"enter"}, Help: "open / confirm"},
}
