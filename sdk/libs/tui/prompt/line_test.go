package prompt

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestLineEditing(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.KeyMsg
		want string
		pos  int
	}{
		{"insert", []tea.KeyMsg{runes("ab"), runes("c")}, "abc", 3},
		{"space is a rune", []tea.KeyMsg{runes("a"), key(tea.KeySpace), runes("b")}, "a b", 3},
		{"unicode", []tea.KeyMsg{runes("héllo")}, "héllo", 5},
		{"backspace", []tea.KeyMsg{runes("abc"), key(tea.KeyBackspace)}, "ab", 2},
		{"backspace at start is a no-op", []tea.KeyMsg{key(tea.KeyBackspace)}, "", 0},
		{"left then insert", []tea.KeyMsg{runes("ac"), key(tea.KeyLeft), runes("b")}, "abc", 2},
		{"delete under cursor", []tea.KeyMsg{runes("abc"), key(tea.KeyHome), key(tea.KeyDelete)}, "bc", 0},
		{"home/end", []tea.KeyMsg{runes("abc"), key(tea.KeyHome), runes("x"), key(tea.KeyEnd), runes("y")}, "xabcy", 5},
		{"ctrl+a / ctrl+e", []tea.KeyMsg{runes("abc"), key(tea.KeyCtrlA), runes("x"), key(tea.KeyCtrlE), runes("y")}, "xabcy", 5},
		{"ctrl+u kills to start", []tea.KeyMsg{runes("abc def"), key(tea.KeyLeft), key(tea.KeyCtrlU)}, "f", 0},
		{"ctrl+w kills a word", []tea.KeyMsg{runes("set install.ai "), key(tea.KeyCtrlW)}, "set ", 4},
		{"right clamps", []tea.KeyMsg{runes("a"), key(tea.KeyRight), key(tea.KeyRight), runes("b")}, "ab", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var l Line
			for _, k := range tc.keys {
				assert.True(t, l.Handle(k), "editing keys are consumed")
			}
			assert.Equal(t, tc.want, l.String())
			assert.Equal(t, tc.pos, l.pos)
		})
	}
}

func TestLineDoesNotConsumeModeKeys(t *testing.T) {
	var l Line
	for _, k := range []tea.KeyType{tea.KeyEscape, tea.KeyEnter, tea.KeyTab, tea.KeyShiftTab, tea.KeyUp, tea.KeyDown} {
		assert.False(t, l.Handle(key(k)), "%v belongs to the owning mode", k)
	}
}

func TestLineRenderResetAtEnd(t *testing.T) {
	var l Line
	l.SetText("abc")
	assert.True(t, l.AtEnd())
	l.Handle(key(tea.KeyLeft))
	assert.False(t, l.AtEnd())
	assert.Equal(t, "ab▌c", l.Render("▌"))
	l.Reset()
	assert.Equal(t, "", l.String())
	assert.Equal(t, "▌", l.Render("▌"))
}
