// Package input splits raw terminal bytes between the focused program and
// mono. Everything goes to the program except mono's prefix key; the key
// right after the prefix is decoded into a bubbletea KeyMsg so it can be
// looked up in the shared sdk keymap like any other sdk TUI key.
package input

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Event is either bytes for the program (Forward) or one key for mono.
type Event struct {
	Forward []byte
	Key     *tea.KeyMsg
}

// Router tracks whether the prefix is waiting for its second key.
type Router struct {
	Prefix  byte
	pending bool
	partial []byte // an escape sequence after the prefix, split across reads
}

// Pending reports whether the prefix was pressed and mono awaits a key.
func (r *Router) Pending() bool { return r.pending }

// Cancel drops a pending prefix.
func (r *Router) Cancel() { r.pending, r.partial = false, nil }

// Feed routes bytes up to and including the first mono key. It returns the
// events and the bytes it did not look at: a mono key can change mono's mode
// (":" opens the prompt), so the caller must route the rest after handling
// the key, not before. With no mono key, rest is empty.
func (r *Router) Feed(b []byte) (events []Event, rest []byte) {
	if len(r.partial) > 0 {
		b = append(r.partial, b...)
		r.partial = nil
	}
	start := 0
	for i := 0; i < len(b); {
		if r.pending {
			if incompleteEscape(b[i:]) {
				// "ESC [" / "ESC O" with the rest still on its way: wait for
				// it rather than decode Esc and leak "[B" to the program.
				r.partial = append([]byte(nil), b[i:]...)
				return events, nil
			}
			k, n := Decode(b[i:])
			r.pending = false
			events = append(events, Event{Key: &k})
			return events, b[i+n:]
		}
		if b[i] == r.Prefix {
			if i > start {
				events = append(events, Event{Forward: append([]byte(nil), b[start:i]...)})
			}
			r.pending = true
			i++
			start = i
			continue
		}
		i++
	}
	if !r.pending && start < len(b) {
		events = append(events, Event{Forward: append([]byte(nil), b[start:]...)})
	}
	return events, nil
}

// incompleteEscape reports whether b is the start of a CSI or SS3 sequence
// whose final byte has not arrived. A lone ESC is complete: it is the Esc key.
func incompleteEscape(b []byte) bool {
	if len(b) < 2 || b[0] != 0x1b {
		return false
	}
	switch b[1] {
	case 'O':
		return len(b) == 2
	case '[':
		for _, c := range b[2:] {
			if c >= 0x40 && c <= 0x7e {
				return false
			}
		}
		return len(b) < 16 // a runaway sequence is decoded (as Esc) rather than held forever
	}
	return false
}

// Keys decodes every key in b (for modes where mono owns the keyboard).
func Keys(b []byte) []tea.KeyMsg {
	var out []tea.KeyMsg
	for len(b) > 0 {
		k, n := Decode(b)
		out = append(out, k)
		b = b[n:]
	}
	return out
}

var csi = map[string]tea.KeyType{
	"A": tea.KeyUp, "B": tea.KeyDown, "C": tea.KeyRight, "D": tea.KeyLeft,
	"H": tea.KeyHome, "F": tea.KeyEnd, "Z": tea.KeyShiftTab,
	"1~": tea.KeyHome, "4~": tea.KeyEnd, "3~": tea.KeyDelete,
	"5~": tea.KeyPgUp, "6~": tea.KeyPgDown, "11~": tea.KeyF1,
}

var ss3 = map[byte]tea.KeyType{
	'A': tea.KeyUp, 'B': tea.KeyDown, 'C': tea.KeyRight, 'D': tea.KeyLeft,
	'H': tea.KeyHome, 'F': tea.KeyEnd, 'P': tea.KeyF1,
}

// Decode reads one key from the front of b and returns it with the number of
// bytes it used (always ≥ 1 for non-empty b). Unknown escape sequences are
// consumed whole and reported as Esc so their bytes never leak as text.
func Decode(b []byte) (tea.KeyMsg, int) {
	c := b[0]
	switch {
	case c == 0x1b:
		if len(b) == 1 {
			return tea.KeyMsg{Type: tea.KeyEscape}, 1
		}
		switch b[1] {
		case '[':
			// CSI: parameters/intermediates until a final byte 0x40–0x7e.
			for j := 2; j < len(b); j++ {
				if b[j] >= 0x40 && b[j] <= 0x7e {
					if t, ok := csi[string(b[2:j+1])]; ok {
						return tea.KeyMsg{Type: t}, j + 1
					}
					return tea.KeyMsg{Type: tea.KeyEscape}, j + 1
				}
			}
			return tea.KeyMsg{Type: tea.KeyEscape}, len(b)
		case 'O':
			if len(b) >= 3 {
				if t, ok := ss3[b[2]]; ok {
					return tea.KeyMsg{Type: t}, 3
				}
				return tea.KeyMsg{Type: tea.KeyEscape}, 3
			}
			return tea.KeyMsg{Type: tea.KeyEscape}, len(b)
		}
		// Alt+key: report the key with Alt set.
		k, n := Decode(b[1:])
		k.Alt = true
		return k, n + 1
	case c == '\r' || c == '\n':
		return tea.KeyMsg{Type: tea.KeyEnter}, 1
	case c == '\t':
		return tea.KeyMsg{Type: tea.KeyTab}, 1
	case c == 0x7f || c == 0x08:
		return tea.KeyMsg{Type: tea.KeyBackspace}, 1
	case c == ' ':
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, 1
	case c == 0:
		return tea.KeyMsg{Type: tea.KeyCtrlAt}, 1
	case c < 0x20:
		// 0x01–0x1a are ctrl+a..ctrl+z; bubbletea's KeyCtrlA..Z equal the byte.
		return tea.KeyMsg{Type: tea.KeyType(c)}, 1
	}
	r, n := utf8.DecodeRune(b)
	if r == utf8.RuneError && n <= 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(c)}}, 1
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, n
}
