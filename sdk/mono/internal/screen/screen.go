// Package screen turns emulated terminal cells into ANSI text and writes
// frames to the real terminal, redrawing only the rows that changed.
package screen

import (
	"fmt"
	"io"
	"strings"

	"github.com/hinshun/vt10x"
	"github.com/mattn/go-runewidth"
)

// Glyph attribute bits, mirroring vt10x's unexported attr* constants.
const (
	attrReverse = 1 << iota
	attrUnderline
	attrBold
	attrGfx
	attrItalic
	attrBlink
)

// Cells is the read side of an emulator (vt10x.View satisfies it).
type Cells interface {
	Cell(x, y int) vt10x.Glyph
	Size() (cols, rows int)
}

// Row renders row y of v as exactly width terminal columns of ANSI text,
// starting and ending with an SGR reset. Cells outside v render as blanks.
//
// Columns, not cells: vt10x keeps a wide rune (emoji, CJK) in one cell, but
// the real terminal draws it two columns wide. Counting cells made gsl's
// emoji line wider than the screen; it wrapped, and on the bottom row the
// terminal scrolled on every frame. A wide rune that would not fit is
// replaced by a blank.
func Row(v Cells, y, width int) string {
	cols, rows := v.Size()
	var b strings.Builder
	b.WriteString("\x1b[0m")
	var cur vt10x.Glyph
	cur.FG, cur.BG = vt10x.DefaultFG, vt10x.DefaultBG
	out := 0
	for x := 0; out < width; x++ {
		g := vt10x.Glyph{Char: ' ', FG: vt10x.DefaultFG, BG: vt10x.DefaultBG}
		if x < cols && y < rows {
			g = v.Cell(x, y)
		}
		if g.Char == 0 {
			g.Char = ' '
		}
		w := runewidth.RuneWidth(g.Char)
		if out+w > width {
			g.Char, w = ' ', 1
		}
		if g.FG != cur.FG || g.BG != cur.BG || g.Mode != cur.Mode {
			b.WriteString(sgr(g))
			cur = g
		}
		b.WriteRune(g.Char)
		out += w
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// sgr is the full SGR sequence for a glyph's style (reset first, so no
// attribute from the previous cell leaks).
func sgr(g vt10x.Glyph) string {
	parts := []string{"0"}
	if g.Mode&attrBold != 0 {
		parts = append(parts, "1")
	}
	if g.Mode&attrItalic != 0 {
		parts = append(parts, "3")
	}
	if g.Mode&attrUnderline != 0 {
		parts = append(parts, "4")
	}
	if g.Mode&attrBlink != 0 {
		parts = append(parts, "5")
	}
	fg, bg := g.FG, g.BG
	if g.Mode&attrReverse != 0 {
		// vt10x already swapped the cell's colors. Swap them back and let the
		// real terminal reverse: that also works for its default colors, which
		// a swapped "default foreground" cannot express.
		fg, bg = bg, fg
		parts = append(parts, "7")
	}
	parts = append(parts, color(fg, false), color(bg, true))
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func color(c vt10x.Color, bg bool) string {
	base, bright, ext, def := 30, 90, "38", "39"
	if bg {
		base, bright, ext, def = 40, 100, "48", "49"
	}
	switch {
	case c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor:
		return def
	case c < 8:
		return fmt.Sprint(base + int(c))
	case c < 16:
		return fmt.Sprint(bright + int(c) - 8)
	case c < 256:
		return fmt.Sprintf("%s;5;%d", ext, c)
	default: // vt10x packs 24-bit color as r<<16|g<<8|b
		return fmt.Sprintf("%s;2;%d;%d;%d", ext, (c>>16)&0xff, (c>>8)&0xff, c&0xff)
	}
}

// Writer draws whole frames to the terminal, rewriting only changed rows.
type Writer struct {
	out  io.Writer
	prev []string
}

// NewWriter wraps the terminal output.
func NewWriter(out io.Writer) *Writer { return &Writer{out: out} }

// Invalidate forces the next Draw to repaint every row (after a resize).
func (w *Writer) Invalidate() { w.prev = nil }

// Cursor is where to leave the terminal cursor; Show=false hides it.
type Cursor struct {
	X, Y int
	Show bool
}

// Draw writes the rows that differ from the previous frame, then places or
// hides the cursor, as one write so the terminal never shows a half frame.
func (w *Writer) Draw(rows []string, cur Cursor) error {
	var b strings.Builder
	b.WriteString("\x1b[?25l") // hide while drawing
	for y, r := range rows {
		if y < len(w.prev) && w.prev[y] == r {
			continue
		}
		fmt.Fprintf(&b, "\x1b[%d;1H%s", y+1, r)
	}
	if cur.Show {
		fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[?25h", cur.Y+1, cur.X+1)
	}
	w.prev = append(w.prev[:0], rows...)
	_, err := io.WriteString(w.out, b.String())
	return err
}
