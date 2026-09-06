// Package nav is the cursor + viewport engine every list TUI needs: clamped
// motion, a viewport that follows the cursor, half/full page strides, and the
// gg chord — all as a value, never a package global.
package nav

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

// Cursor tracks the selected row and the visible window over Len rows.
type Cursor struct {
	Pos, Len    int
	Top, Height int
	pendingG    bool
}

// SetLen updates the row count and clamps the cursor.
func (c *Cursor) SetLen(n int) {
	c.Len = n
	c.To(c.Pos)
}

// SetHeight updates the visible row budget and re-clamps the viewport.
func (c *Cursor) SetHeight(h int) {
	c.Height = h
	c.Clamp()
}

// To moves to row i, clamped to the list, and keeps it visible.
func (c *Cursor) To(i int) {
	if c.Len <= 0 {
		c.Pos, c.Top = 0, 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i > c.Len-1 {
		i = c.Len - 1
	}
	c.Pos = i
	c.Clamp()
}

// Move moves by delta rows.
func (c *Cursor) Move(delta int) { c.To(c.Pos + delta) }

// Clamp enforces Top <= Pos < Top+Height and Top <= max(0, Len-Height)
// (extracted from fleet's clampViewport).
func (c *Cursor) Clamp() {
	h := c.Height
	if h < 1 {
		c.Top = 0
		return
	}
	if c.Pos < c.Top {
		c.Top = c.Pos
	}
	if c.Pos >= c.Top+h {
		c.Top = c.Pos - h + 1
	}
	if max := c.Len - h; c.Top > max {
		c.Top = max
	}
	if c.Top < 0 {
		c.Top = 0
	}
}

// Visible is the [start, end) row range to render.
func (c *Cursor) Visible() (int, int) {
	if c.Len <= 0 {
		return 0, 0
	}
	end := c.Len
	if c.Height > 0 && c.Top+c.Height < end {
		end = c.Top + c.Height
	}
	return c.Top, end
}

// Half is the ctrl+d / ctrl+u stride.
func (c *Cursor) Half() int {
	if c.Height <= 0 {
		return 5
	}
	if n := c.Height / 2; n > 0 {
		return n
	}
	return 1
}

// Page is the ctrl+f / ctrl+b stride.
func (c *Cursor) Page() int {
	if c.Height <= 0 {
		return 10
	}
	return c.Height
}

// Apply performs a motion action; false means "not a motion".
func (c *Cursor) Apply(a keymap.Action) bool {
	switch a {
	case keymap.Up:
		c.Move(-1)
	case keymap.Down:
		c.Move(1)
	case keymap.Top:
		c.To(0)
	case keymap.Bottom:
		c.To(c.Len - 1)
	case keymap.HalfUp:
		c.Move(-c.Half())
	case keymap.HalfDown:
		c.Move(c.Half())
	case keymap.PageUp:
		c.Move(-c.Page())
	case keymap.PageDown:
		c.Move(c.Page())
	default:
		return false
	}
	return true
}

// Pending reports whether a "g" is waiting for its partner.
func (c *Cursor) Pending() bool { return c.pendingG }

// Key routes a key event: the gg chord first (only when the map binds "gg"),
// then motion actions. A non-motion key cancels a pending g and is NOT
// consumed, so the caller handles it normally.
func (c *Cursor) Key(msg tea.KeyMsg, m keymap.Map) bool {
	name := keymap.KeyName(msg)
	if c.pendingG {
		c.pendingG = false
		if name == "g" {
			c.To(0)
			return true
		}
	} else if name == "g" && m.Has("gg") {
		c.pendingG = true
		return true
	}
	if a, ok := m.Lookup(msg); ok {
		return c.Apply(a)
	}
	return false
}
