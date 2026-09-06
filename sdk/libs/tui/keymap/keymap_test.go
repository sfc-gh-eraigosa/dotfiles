package keymap

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }
func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestLookupUsesRealKeyNames(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want Action
	}{
		{r("j"), Down}, {k(tea.KeyDown), Down}, {r("k"), Up},
		{r("h"), PageLeft}, {k(tea.KeyRight), PageRight},
		{r("G"), Bottom}, {k(tea.KeyCtrlD), HalfDown}, {k(tea.KeyCtrlU), HalfUp},
		{k(tea.KeyCtrlF), PageDown}, {k(tea.KeyPgDown), PageDown}, {k(tea.KeyCtrlB), PageUp}, {k(tea.KeyPgUp), PageUp},
		{r("/"), Search}, {r("n"), NextMatch}, {r("N"), PrevMatch}, {k(tea.KeyEscape), ClearHighlight},
		{r(":"), Command}, {r("?"), Help}, {k(tea.KeyF1), Help}, {r("q"), Quit}, {k(tea.KeyCtrlC), Quit},
		{k(tea.KeySpace), Select}, {r(" "), Select}, {k(tea.KeyEnter), Confirm},
	}
	for _, tc := range cases {
		got, ok := Vim.Lookup(tc.msg)
		require.True(t, ok, "%s must be bound", tc.msg)
		assert.Equal(t, tc.want, got, tc.msg.String())
	}
	_, ok := Vim.Lookup(r("z"))
	assert.False(t, ok)
	_, ok = Vim.Lookup(r("g")) // gg is a chord owned by nav, not a Lookup hit
	assert.False(t, ok)
	assert.True(t, Vim.Has("gg"))
}

func TestMergeReplacesInPlaceOrAppends(t *testing.T) {
	m := Vim.Merge(
		Binding{Action: PageRight, Keys: []string{"l"}, Help: "open the log pane"},
		Binding{Action: "refresh", Keys: []string{"r"}, Help: "refresh"},
	)
	assert.Equal(t, len(Vim)+1, len(m))
	i := indexOf(m, PageRight)
	assert.Equal(t, indexOf(Vim, PageRight), i, "replaced in place")
	assert.Equal(t, "open the log pane", m[i].Help)
	assert.Equal(t, "refresh", string(m[len(m)-1].Action))
	assert.NotEqual(t, "open the log pane", Vim[indexOf(Vim, PageRight)].Help, "Vim is never mutated")
}

func TestWithoutRemovesActions(t *testing.T) {
	m := Vim.Without(PageLeft, PageRight, "absent")
	assert.Equal(t, len(Vim)-2, len(m))
	_, ok := m.Lookup(r("h"))
	assert.False(t, ok)
	assert.Nil(t, m.Keys(PageLeft))
	assert.Equal(t, []string{"j", "down"}, m.Keys(Down))
}

func TestHeaderHintGroupsAndOrders(t *testing.T) {
	m := Map{
		{Action: Down, Keys: []string{"j", "down"}, Short: "move", Group: "move", Header: true},
		{Action: Up, Keys: []string{"k", "up"}, Short: "move", Group: "move", Header: true},
		{Action: Top, Keys: []string{"gg"}, Help: "first row"},
		{Action: Search, Keys: []string{"/"}, Help: "regex search", Short: "search", Header: true},
		{Action: Quit, Keys: []string{"q", "ctrl+c"}, Help: "quit", Header: true},
	}
	assert.Equal(t, "j/k move  / search  q quit", m.HeaderHint("  "))
	assert.Equal(t, "", Map{}.HeaderHint("  "))
	assert.Equal(t, "j/k move  h/l page  / search  n/N match  : command  ? help  q quit", Vim.HeaderHint("  "))
}

func TestHelpRowsJoinKeys(t *testing.T) {
	rows := Vim.HelpRows()
	assert.Equal(t, len(Vim), len(rows))
	assert.Equal(t, "j/down", rows[indexOf(Vim, Down)].Keys)
	assert.Equal(t, "gg", rows[indexOf(Vim, Top)].Keys)
}

func TestDispatchCallsTheBoundHandlerOnce(t *testing.T) {
	calls := 0
	h := Handlers{Quit: func() tea.Cmd { calls++; return tea.Quit }}
	handled, cmd := Dispatch(Vim, r("q"), h)
	assert.True(t, handled)
	assert.NotNil(t, cmd)
	assert.Equal(t, 1, calls)
	handled, _ = Dispatch(Vim, r("j"), h) // bound, no handler
	assert.False(t, handled)
	handled, _ = Dispatch(Vim, r("z"), h) // unbound
	assert.False(t, handled)
}

func indexOf(m Map, a Action) int {
	for i, b := range m {
		if b.Action == a {
			return i
		}
	}
	return -1
}
