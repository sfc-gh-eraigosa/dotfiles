package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// keyHelp is the single source of truth for the keymap: the help overlay
// renders from it, so there is no second hand-written list to drift.
// keyHelp is the single source of truth for the keymap: both the always-visible
// header strip and the `?` overlay render from it, so a key can never again be
// implemented and documented in the overlay while staying invisible on screen
// (which is exactly how the log pane shipped undiscoverable).
//
// icon is a visual anchor next to the letter — the letter stays authoritative,
// the icon just makes the strip scannable. hdr marks the few that earn a place
// in the always-on header; the rest live in the overlay.
var keyHelp = []struct {
	icon, keys, what string
	hdr              bool
}{
	{"❓", "?", "toggle this help", true},
	{"🔍", "/", "regex search (smartcase)", true},
	{"●", "space", "toggle selection", true},
	{"🚀", "u", "update selection (or cursor host)", true},
	{"📜", "l", "show / hide the streaming log pane", true},
	{"🖥️", "s", "ssh to cursor host", true},
	{"🔄", "r", "refresh", true},
	{"🚪", "q", "quit", true},
	{"⬍", "j / k / ↓ / ↑", "move cursor", false},
	{"⤒", "gg / G", "first / last host", false},
	{"⇟", "ctrl+d / ctrl+u", "half page down / up", false},
	{"⇞", "ctrl+f / ctrl+b", "page down / up", false},
	{"➡️", "n / N", "next / previous match", false},
	{"◉", "a", "select all (respects an active search)", false},
	{"📖", "J / K", "scroll the log pane (G re-follows the tail)", false},
	{"◍", "v", "visual range select", false},
	{"⎋", "esc", "clear search / selection", false},
	{"⏰", "w", "wake selection (or cursor host)", false},
	{"🗑️", "F", "forget answers (incl. saved preferences)", false},
	{"⇥", "tab / enter", "(answer form) next field · esc backs out, keeping answers", false},
	{"✏️", "e", "(confirm) edit the remembered answers · enter runs the update", false},
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
	case fieldReset:
		switch k.String() {
		case "y", "n":
			m.ans.reset = k.String()
		case " ", "right", "left":
			m.ans.reset = cycle(m.ans.reset, []string{"", "n", "y"}, k.String() != "left")
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
	// The focused pane owns the pattern, so searching the log never disturbs a
	// host filter the operator set earlier (and vice versa).
	inLog := m.logFocus && m.logOpen
	st := &m.search
	if inLog {
		st = &m.logSearch
	}
	switch k.String() {
	case "esc":
		*st = searchState{}
		m.mode = modeNormal
	case "enter":
		st.committed = true
		m.mode = modeNormal
		if st.re != nil {
			if inLog {
				m.logJump(1)
			} else {
				m.jumpMatch(1)
			}
		}
	case "backspace":
		if n := len(st.input); n > 0 {
			st.input = st.input[:n-1]
			compileInto(st)
		}
	default:
		if r := k.Runes; len(r) > 0 {
			st.input += string(r)
			compileInto(st)
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
	case "enter", "y", "Y":
		// enter is the primary: it is what the answer form hands off to, so the
		// whole flow ends on the same key it was driven with.
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
			// gg goes to the top of whichever pane has the keys.
			if m.logFocus && m.logOpen {
				m.logTo(0)
			} else {
				m.moveTo(0)
			}
			return m, nil
		}
		// fall through: the pending g is cancelled, this key acts normally
	}

	// With the log focused, the vim keys drive it instead of the host list, so
	// a long install log is navigable with the same muscle memory.
	if m.logFocus && m.logOpen {
		switch key {
		case "j", "down":
			m.logTo(m.logTop + 1)
			return m, nil
		case "k", "up":
			m.logTo(m.logTop - 1)
			return m, nil
		case "ctrl+d":
			m.logTo(m.logTop + maxInt(1, m.logHeight()/2))
			return m, nil
		case "ctrl+u":
			m.logTo(m.logTop - maxInt(1, m.logHeight()/2))
			return m, nil
		case "ctrl+f", "pgdown":
			m.logTo(m.logTop + maxInt(1, m.logHeight()))
			return m, nil
		case "ctrl+b", "pgup":
			m.logTo(m.logTop - maxInt(1, m.logHeight()))
			return m, nil
		case "g":
			pendingG = true
			return m, nil
		case "G":
			// Jumping to the end means "show me the newest", which is exactly
			// what following does — so G resumes it rather than pinning a
			// stale offset that a still-running install would scroll past.
			m.logFollow = true
			return m, nil
		case "/":
			m.mode = modeSearch
			m.logSearch = searchState{}
			return m, nil
		case "n":
			m.logJump(1)
			return m, nil
		case "N":
			m.logJump(-1)
			return m, nil
		}
	}

	switch key {
	case "tab":
		// Only meaningful when there is a log to focus.
		if m.logOpen {
			m.logFocus = !m.logFocus
		}
		return m, nil
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
			// An empty credential ALWAYS prompts forward, even when the other
			// answers are remembered. Otherwise a session that had only ever
			// set `windows` counted as "remembered" and jumped straight to
			// confirm — so the wave ran with no credential, every privileged
			// step silently skipped, and the operator was never asked for the
			// one answer that cannot be defaulted.
			if m.ans.secretLen() == 0 || !m.ans.remembered() {
				m.ansField = fieldSudo
				m.mode = modeAnswers
			} else {
				m.mode = modeConfirm // `e` from there edits without discarding
			}
		}
	case "l":
		// Toggling off restores the host list to the full viewport.
		m.logOpen = !m.logOpen
		m.logFollow = true
		m.clampViewport()
	case "J":
		// Scrolling stops the tail from yanking the view away mid-read.
		if m.logOpen {
			m.logFollow = false
			m.logTop = minInt(m.logTop+1, maxInt(0, len(m.logs)-1))
		}
	case "K":
		if m.logOpen {
			m.logFollow = false
			m.logTop = maxInt(0, m.logTop-1)
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
