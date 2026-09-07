package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func rn(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestVimJKMoveLikeArrows(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand install
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.claude")
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.teams")
	m = press(m, rn("k"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.claude")
}

func TestVimKClampsAtTop(t *testing.T) {
	m := newPagerModel(t)
	before := m.View()
	m = press(m, rn("k"))
	assert.Equal(t, before, m.View(), "k on the first row is a no-op")
}

func TestVimHLTurnPages(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, rn("l"))
	assert.Contains(t, m.View(), "[ai]", "l = next category page")
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "(All)]", "h = previous page")
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "[shell]", "h wraps to the last page")
}

func TestVimGGAndG(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, rn("G"))
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "G = last row")
	m = press(m, rn("g"))
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "a lone g moves nothing")
	m = press(m, rn("g"))
	assert.NotContains(t, cursorLine(m.View()), "install.", "gg = first row (the area header)")
}

func TestVimPendingGIsCancelledByAnotherKey(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, rn("G"))
	m = press(m, rn("g"))
	m = press(m, rn("k")) // cancels the pending g AND moves up once
	assert.Contains(t, cursorLine(m.View()), "install.pkg.manager")
}

func TestVimCtrlDUHalfPage(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.View() // establishes the body height
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	assert.NotContains(t, cursorLine(m.View()), "▼ install", "ctrl+d moved off the header")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	assert.Contains(t, cursorLine(m.View()), "install", "ctrl+u returns to the top")
}

func TestVimCtrlFBFullPage(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlF})
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "ctrl+f clamps to the last row on a short list")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlB})
	assert.NotContains(t, cursorLine(m.View()), "install.", "ctrl+b clamps to the first row")
}

func TestHelpOpensOnQuestionAndF1NotH(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, rn("h"))
	assert.NotContains(t, m.View(), "KEYS", "h no longer opens help in the list")
	m = press(m, tea.KeyMsg{Type: tea.KeyF1})
	assert.Contains(t, m.View(), "KEYS", "F1 opens help")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	m = press(m, rn("?"))
	v := m.View()
	assert.Contains(t, v, "KEYS", "? opens help")
	for _, want := range []string{"j/down", "gg", "ctrl+d", "/", ":", "q/Q/ctrl+c", "space", "u "} {
		assert.Contains(t, v, want, "help lists %q from the keymap", want)
	}
	assert.Contains(t, v, "SOURCES", "gff's own section still renders")
}

func TestHDoesNotOpenHelpInPickerOrDetail(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg page
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // picker
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "Pick option", "h in the picker is inert")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "LAYERS", "h in the detail view is inert")
}

func TestPickerJKMoveCursor(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "apt", "j moves the picker cursor")
	m = press(m, rn("k"))
	assert.Contains(t, cursorLine(m.View()), "auto", "k moves it back")
}

func TestFooterHintRendersFromTheKeymap(t *testing.T) {
	m := newPagerModel(t)
	v := m.View()
	for _, want := range []string{"j/k move", "h/l page", "/ search", "n/N match", ": command", "? help", "q quit", "space toggle", "enter open", "u clear"} {
		assert.Contains(t, v, want)
	}
}

// Review #304 finding 9: the detail legend this PR wrote advertises "?/F1 this
// help", so F1 must close the overlay it opens — not only ?/q/Esc.
func TestF1TogglesTheHelpOverlayShut(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyF1})
	assert.Contains(t, m.View(), "KEYS", "F1 opens help")
	m = press(m, tea.KeyMsg{Type: tea.KeyF1})
	assert.NotContains(t, m.View(), "KEYS", "F1 closes it again")
}

// Review #304 finding 10: 'Q' quit before this PR and 'u'/'U' both still
// clear an override, so the uppercase alias must survive the keymap move.
func TestUppercaseQStillQuits(t *testing.T) {
	m := newPagerModel(t)
	_, cmd := m.Update(rn("Q"))
	assert.NotNil(t, cmd, "Q quits like q")
}

// Review #304 finding 5: the list legend grew from 4 hand-written lines to the
// full 20-row key table, so the overlay has to respect the window like the
// list does.
func TestHelpOverlayFitsTheWindow(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(m, rn("?"))
	v := m.View()
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 24, "the overlay fits an 80x24 terminal")
	assert.Contains(t, v, "gff — git fast features", "the title survives the trim")
	assert.Contains(t, v, "close", "so does the close hint")
}
