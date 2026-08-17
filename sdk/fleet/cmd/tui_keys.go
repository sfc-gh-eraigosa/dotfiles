package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
)

// keyHelp is the single source of truth for the keymap: the help overlay
// renders from it, so there is no second hand-written list to drift.
var keyHelp = []struct{ keys, what string }{
	{"j / k / ↓ / ↑", "move cursor"},
	{"gg / G", "first / last host"},
	{"ctrl+d / ctrl+u", "half page down / up"},
	{"ctrl+f / ctrl+b", "page down / up"},
	{"/", "regex search (smartcase)"},
	{"n / N", "next / previous match"},
	{"space", "toggle selection"},
	{"v", "visual range select"},
	{"esc", "clear search / selection"},
	{"u", "update selection (or cursor host)"},
	{"s", "ssh to cursor host"},
	{"r", "refresh"},
	{"?", "toggle this help"},
	{"q", "quit"},
}

// route owns every keystroke. Mode comes first: a key typed in search is text,
// not a motion — the bug class this structure exists to prevent.
func route(m tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		m.mode = modeNormal // any key closes
		return m, nil
	case modeSearch:
		return routeSearch(m, k)
	case modeConfirm:
		return routeConfirm(m, k)
	}
	return routeNormal(m, k)
}

func routeSearch(m tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.search = searchState{}
		m.mode = modeNormal
	case "enter":
		m.search.committed = true
		m.mode = modeNormal
		if m.search.re != nil {
			m.jumpMatch(1)
		}
	case "backspace":
		if n := len(m.search.input); n > 0 {
			m.search.input = m.search.input[:n-1]
			m.compileSearch()
		}
	default:
		if r := k.Runes; len(r) > 0 {
			m.search.input += string(r)
			m.compileSearch()
		}
	}
	return m, nil
}

func routeConfirm(m tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y", "enter":
		targets := m.updateTargets()
		m.mode = modeNormal
		return m, m.startUpdate(targets)
	default: // n, esc, anything else — declining must change nothing
		m.mode = modeNormal
		m.status = "update cancelled"
		return m, nil
	}
}

// pendingG tracks the first `g` of a `gg` sequence. Any other key cancels it.
var pendingG bool

func routeNormal(m tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()

	if pendingG {
		pendingG = false
		if key == "g" {
			m.moveTo(0)
			return m, nil
		}
		// fall through: the pending g is cancelled, this key acts normally
	}

	switch key {
	case "q", "ctrl+c":
		// Quitting mid-update would orphan work the operator can't see.
		if m.busy() && !m.quitReq {
			m.quitReq = true
			m.status = "updates in progress — press q again to force quit"
			return m, nil
		}
		return m, tea.Quit
	case "g":
		pendingG = true
	case "G":
		m.moveTo(len(m.rows) - 1)
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d":
		m.move(m.visibleRows() / 2)
	case "ctrl+u":
		m.move(-m.visibleRows() / 2)
	case "ctrl+f", "pgdown":
		m.move(m.visibleRows())
	case "ctrl+b", "pgup":
		m.move(-m.visibleRows())
	case "/":
		m.mode = modeSearch
		m.search = searchState{}
	case "n":
		m.jumpMatch(1)
	case "N":
		m.jumpMatch(-1)
	case " ":
		if a, b, ok := m.visualRange(); ok {
			for i := a; i <= b; i++ {
				m.selected[m.rows[i].Alias] = true
			}
			m.vAnchor = nil
		} else if m.cursor != "" {
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
		}
	case "v":
		if m.cursor != "" {
			c := m.cursor
			m.vAnchor = &c
		}
	case "esc":
		m.vAnchor = nil
		m.selected = map[string]bool{}
		m.search = searchState{}
	case "u":
		if len(m.updateTargets()) > 0 {
			m.mode = modeConfirm
		}
	case "s":
		// An ssh visit while the engine owns the host would race its update.
		if m.cursor != "" && !m.inFlight(m.cursor) {
			return m, sshShell(m.cursor)
		}
	case "r":
		return m, m.refresh()
	case "?":
		m.mode = modeHelp
	}
	m.clampViewport()
	return m, nil
}
