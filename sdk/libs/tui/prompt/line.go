// Package prompt is the single-line editor behind / and : prompts.
package prompt

import tea "github.com/charmbracelet/bubbletea"

// Line is an editable rune buffer with a cursor. It owns only editing keys;
// Esc/Enter/Tab/Shift-Tab/Up/Down are the owning mode's business.
type Line struct {
	runes []rune
	pos   int
}

func (l *Line) String() string { return string(l.runes) }

func (l *Line) Reset() { l.runes, l.pos = l.runes[:0], 0 }

// SetText replaces the buffer and parks the cursor at the end.
func (l *Line) SetText(s string) { l.runes = []rune(s); l.pos = len(l.runes) }

// AtEnd reports whether the cursor sits after the last rune (completion only
// makes sense there).
func (l *Line) AtEnd() bool { return l.pos == len(l.runes) }

// Render is the text with the cursor glyph at the insertion point.
func (l *Line) Render(cursor string) string {
	return string(l.runes[:l.pos]) + cursor + string(l.runes[l.pos:])
}

// Handle applies one editing key and reports whether it consumed it.
func (l *Line) Handle(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		l.insert(msg.Runes)
	case tea.KeySpace:
		l.insert([]rune{' '})
	case tea.KeyBackspace:
		if l.pos > 0 {
			l.runes = append(l.runes[:l.pos-1], l.runes[l.pos:]...)
			l.pos--
		}
	case tea.KeyDelete:
		if l.pos < len(l.runes) {
			l.runes = append(l.runes[:l.pos], l.runes[l.pos+1:]...)
		}
	case tea.KeyLeft:
		if l.pos > 0 {
			l.pos--
		}
	case tea.KeyRight:
		if l.pos < len(l.runes) {
			l.pos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		l.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		l.pos = len(l.runes)
	case tea.KeyCtrlU:
		l.runes = append([]rune{}, l.runes[l.pos:]...)
		l.pos = 0
	case tea.KeyCtrlW:
		start := l.pos
		for start > 0 && l.runes[start-1] == ' ' {
			start--
		}
		for start > 0 && l.runes[start-1] != ' ' {
			start--
		}
		l.runes = append(l.runes[:start], l.runes[l.pos:]...)
		l.pos = start
	default:
		return false
	}
	return true
}

func (l *Line) insert(rs []rune) {
	out := make([]rune, 0, len(l.runes)+len(rs))
	out = append(out, l.runes[:l.pos]...)
	out = append(out, rs...)
	out = append(out, l.runes[l.pos:]...)
	l.runes = out
	l.pos += len(rs)
}
