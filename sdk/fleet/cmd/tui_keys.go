package cmd

import (
	"os"

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
	{"a", "select all (respects an active search)"},
	{"v", "visual range select"},
	{"esc", "clear search / selection"},
	{"u", "update selection (or cursor host)"},
	{"w", "wake selection (or cursor host)"},
	{"F", "forget answers (incl. saved preferences)"},
	{"tab / enter", "(answer form) next field · esc backs out, keeping answers"},
	{"e", "(confirm) edit the remembered answers"},
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
	case modeAnswers:
		return routeAnswers(m, k)
	case modeConfirm:
		return routeConfirm(m, k)
	}
	return routeNormal(m, k)
}

// routeAnswers drives the pre-wave form. It is deliberately its own mode: the
// sudo field accepts arbitrary text (including 'j', 'y', 'q'…) and none of it
// may leak into a normal-mode action.
func routeAnswers(m tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		// Backs OUT of the form; it does NOT forget. Answers persist for the
		// session so a fleet-wide update applies the same ones to every wave —
		// `F` in normal mode is the deliberate way to forget them.
		m.mode = modeNormal
		m.status = "update cancelled"
		return m, nil
	case "tab", "down":
		m.ansField = (m.ansField + 1) % answerFieldCount
		return m, nil
	case "shift+tab", "up":
		m.ansField = (m.ansField + answerFieldCount - 1) % answerFieldCount
		return m, nil
	case "enter":
		// Advance field by field; committing from the last one opens the
		// confirm strip, so there is always one final look before anything runs.
		if m.ansField < answerFieldCount-1 {
			m.ansField++
			return m, nil
		}
		// Persist the non-secret preferences as the form commits, so the next
		// session starts where this one left off. A failed write is not worth
		// interrupting a wave for — it costs a retype next time, nothing more.
		_ = saveAnswers(m.ansPath, m.ans)
		m.mode = modeConfirm
		return m, nil
	}

	switch m.ansField {
	case fieldSudo:
		if k.String() == "backspace" {
			m.ans.trimSecret()
		} else if r := k.Runes; len(r) > 0 {
			m.ans.appendSecret(string(r))
		}
	case fieldWindows:
		// Fixed choices, not free text: these map onto install.sh's own y/n/s.
		switch k.String() {
		case "y", "n", "s":
			m.ans.windows = k.String()
		case " ", "right", "left":
			m.ans.windows = cycle(m.ans.windows, []string{"", "n", "s", "y"}, k.String() != "left")
		}
	case fieldGemini:
		switch k.String() {
		case "y":
			m.ans.gemini = "yes"
		case "k":
			m.ans.gemini = "keep"
		case "n":
			m.ans.gemini = "skip"
		case " ", "right", "left":
			m.ans.gemini = cycle(m.ans.gemini, []string{"", "skip", "keep", "yes"}, k.String() != "left")
		}
	}
	return m, nil
}

// cycle steps through a fixed option ring in either direction.
func cycle(cur string, ring []string, fwd bool) string {
	i := 0
	for j, v := range ring {
		if v == cur {
			i = j
			break
		}
	}
	if fwd {
		return ring[(i+1)%len(ring)]
	}
	return ring[(i+len(ring)-1)%len(ring)]
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
	case "e":
		// Correct a remembered set without throwing it away first.
		m.ansField = fieldSudo
		m.mode = modeAnswers
		return m, nil
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
		// The form is asked once per session, not once per wave: retyping a
		// credential for every wave is what made a fleet-wide update apply
		// INCONSISTENT answers. Staleness is guarded by the confirm strip,
		// which shows exactly what is about to be applied.
		if len(m.updateTargets()) > 0 {
			if m.ans.remembered() {
				m.mode = modeConfirm
			} else {
				m.ansField = fieldSudo
				m.mode = modeAnswers
			}
		}
	case "a":
		m.selectAllFiltered()
	case "F":
		// esc no longer forgets, so forgetting is explicit. Normal mode only:
		// in search or the answer form an F is a character the operator typed.
		// "Forget" means forget: the saved preferences go too, or a restart
		// would quietly bring back what the operator just discarded.
		m.ans = answers{}
		if m.ansPath != "" {
			_ = os.Remove(m.ansPath)
		}
		m.status = "answers forgotten"
	case "w":
		// Wake needs no confirm strip: it mutates nothing on the target, so
		// the worst case of a stray `w` is a few seconds of ICMP.
		if t := m.updateTargets(); len(t) > 0 {
			return m, m.startWake(t)
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
