package nav

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/stretchr/testify/assert"
)

func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestMoveClampsBothEnds(t *testing.T) {
	c := Cursor{Len: 5, Height: 3}
	c.Move(-1)
	assert.Equal(t, 0, c.Pos)
	c.Move(10)
	assert.Equal(t, 4, c.Pos)
	c.Move(-2)
	assert.Equal(t, 2, c.Pos)
}

func TestEmptyListNeverPanics(t *testing.T) {
	var c Cursor
	c.Move(1)
	c.To(7)
	c.Key(r("G"), keymap.Vim)
	c.Key(r("g"), keymap.Vim)
	c.Key(r("g"), keymap.Vim)
	assert.Equal(t, 0, c.Pos)
	assert.Equal(t, 0, c.Top)
	s, e := c.Visible()
	assert.Equal(t, 0, s)
	assert.Equal(t, 0, e)
}

func TestClampKeepsCursorOnScreen(t *testing.T) {
	c := Cursor{Len: 10, Height: 3}
	c.To(5)
	assert.Equal(t, 3, c.Top, "viewport scrolls so Pos is the last visible row")
	c.To(1)
	assert.Equal(t, 1, c.Top, "scrolls back up to Pos")
	c.To(9)
	assert.Equal(t, 7, c.Top, "Top never exceeds Len-Height")
	c.SetLen(4)
	assert.Equal(t, 3, c.Pos, "SetLen clamps Pos")
	assert.Equal(t, 1, c.Top)
	s, e := c.Visible()
	assert.Equal(t, 1, s)
	assert.Equal(t, 4, e)
}

func TestStridesFallBackWhenHeightUnknown(t *testing.T) {
	var c Cursor
	assert.Equal(t, 5, c.Half())
	assert.Equal(t, 10, c.Page())
	c.SetHeight(1)
	assert.Equal(t, 1, c.Half())
	assert.Equal(t, 1, c.Page())
	c.SetHeight(8)
	assert.Equal(t, 4, c.Half())
	assert.Equal(t, 8, c.Page())
}

func TestApplyActions(t *testing.T) {
	c := Cursor{Len: 30, Height: 8}
	assert.True(t, c.Apply(keymap.HalfDown))
	assert.Equal(t, 4, c.Pos)
	assert.True(t, c.Apply(keymap.PageDown))
	assert.Equal(t, 12, c.Pos)
	assert.True(t, c.Apply(keymap.Bottom))
	assert.Equal(t, 29, c.Pos)
	assert.True(t, c.Apply(keymap.PageUp))
	assert.Equal(t, 21, c.Pos)
	assert.True(t, c.Apply(keymap.HalfUp))
	assert.Equal(t, 17, c.Pos)
	assert.True(t, c.Apply(keymap.Top))
	assert.Equal(t, 0, c.Pos)
	assert.True(t, c.Apply(keymap.Down))
	assert.True(t, c.Apply(keymap.Up))
	assert.Equal(t, 0, c.Pos)
	assert.False(t, c.Apply(keymap.Search), "non-motion actions are not consumed")
}

func TestGGChordAndCancellation(t *testing.T) {
	c := Cursor{Len: 10, Height: 5}
	c.To(9)
	assert.True(t, c.Key(r("g"), keymap.Vim))
	assert.True(t, c.Pending())
	assert.Equal(t, 9, c.Pos, "a lone g moves nothing")
	assert.True(t, c.Key(r("g"), keymap.Vim))
	assert.Equal(t, 0, c.Pos, "gg = top")
	assert.False(t, c.Pending())

	c.To(9)
	assert.True(t, c.Key(r("g"), keymap.Vim))
	assert.True(t, c.Key(r("k"), keymap.Vim), "k cancels the pending g AND moves")
	assert.Equal(t, 8, c.Pos)
	assert.False(t, c.Pending())

	c.Key(r("g"), keymap.Vim)
	assert.False(t, c.Key(r("/"), keymap.Vim), "a non-motion key cancels the chord and is NOT consumed")
	assert.False(t, c.Pending())
}

func TestKeyWithoutChordInMap(t *testing.T) {
	m := keymap.Vim.Without(keymap.Top)
	c := Cursor{Len: 3}
	assert.False(t, c.Key(r("g"), m), "no gg binding → g is not a chord")
	assert.True(t, c.Key(k(tea.KeyDown), m))
	assert.Equal(t, 1, c.Pos)
}
