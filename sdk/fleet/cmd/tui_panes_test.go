package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPaneDefaults(t *testing.T) {
	m := testModel("a")
	if !m.hostOpen || !m.logOpen || m.errOpen {
		t.Fatalf("defaults are host+log on, error off: %v %v %v", m.hostOpen, m.logOpen, m.errOpen)
	}
}

func TestHostPaneTogglesAndRestores(t *testing.T) {
	m := testModel("a", "b", "c")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	full := mm.(tuiModel)
	before, cursor := full.visibleRows(), full.cursor

	hidden, _ := send(full, "h")
	if hidden.hostOpen {
		t.Fatal("`h` must hide the host pane")
	}
	if hidden.visibleRows() != 0 {
		t.Fatalf("a hidden host pane reserves no rows, got %d", hidden.visibleRows())
	}

	back, _ := send(hidden, "h")
	if !back.hostOpen || back.visibleRows() != before || back.cursor != cursor {
		t.Fatalf("`h h` must restore height %d and cursor %q, got %d/%q",
			before, cursor, back.visibleRows(), back.cursor)
	}
}

func TestErrorPaneTogglesAndDoesNotStealConfirmEdit(t *testing.T) {
	m := testModel("a")
	on, _ := send(m, "e")
	if !on.errOpen {
		t.Fatal("`e` must open the error pane in normal mode")
	}
	// `e` in the confirm strip still means "edit the answers".
	c := testModel("a")
	c.ans = answers{sudoSecret: "xx"}
	c.mode = modeConfirm
	edited, _ := send(c, "e")
	if edited.mode != modeAnswers {
		t.Fatalf("`e` in modeConfirm must still edit answers, mode=%v", edited.mode)
	}
	if edited.errOpen {
		t.Fatal("`e` in modeConfirm must NOT toggle the error pane")
	}
}

func TestHidingTheLastPaneIsRefused(t *testing.T) {
	m := testModel("a")
	m.logOpen, m.errOpen = false, false // host is the only one left
	got, _ := send(m, "h")
	if !got.hostOpen {
		t.Fatal("the last visible pane must not be hideable")
	}
	if got.status == "" {
		t.Fatal("the refusal must say why")
	}
	again, _ := send(got, "h")
	if !again.hostOpen {
		t.Fatal("still refused")
	}
}

func TestFocusCyclesVisiblePanesOnly(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "x", false)
	m.hostOpen, m.logOpen, m.errOpen = true, true, false

	one, _ := send(m, "tab")
	if one.focus != paneLog {
		t.Fatalf("tab: host -> log, got %v", one.focus)
	}
	two, _ := send(one, "tab")
	if two.focus != paneHost {
		t.Fatalf("tab must skip the hidden error pane, got %v", two.focus)
	}

	// Hiding the focused pane moves focus somewhere visible.
	three := two
	three.focus = paneLog
	hid, _ := send(three, "l")
	if hid.focus == paneLog {
		t.Fatal("focus must leave a pane that was just hidden")
	}
}

func TestPerPaneSearchIsIndependent(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "installing", false)
	m.appendLogLine("a", "WARNING: apt-get update failed", true)
	m.errOpen, m.focus = true, paneErr

	s, _ := send(m, "/", "a", "p", "t", "enter")

	if s.errSearch.input != "apt" {
		t.Fatalf("the error pane owns its pattern, got %q", s.errSearch.input)
	}
	if s.search.input != "" || s.logSearch.input != "" {
		t.Fatal("searching the error pane must not disturb the host filter or the log")
	}
}

func TestPaneStateIsNotPersisted(t *testing.T) {
	a := testModel("x")
	a.hostOpen, a.errOpen = false, true
	b := testModel("x")
	if !b.hostOpen || b.errOpen {
		t.Fatal("a new model always starts at the defaults; pane state is session-only")
	}
}

func TestPaneKeysAreDeclaredInKeyHelp(t *testing.T) {
	rows := map[string]int{}
	hdr := map[string]bool{}
	for _, k := range keyHelp {
		rows[k.keys]++
		hdr[k.keys] = hdr[k.keys] || k.hdr
	}
	for _, key := range []string{"h", "e", "l"} {
		// EXACTLY one: `e` already had a row for the confirm-mode meaning, and
		// the pane toggle shares it rather than adding a second (a duplicate
		// fails the pre-existing TestKeyHelpHasNoDuplicateBindings).
		if rows[key] != 1 {
			t.Errorf("keyHelp must have exactly one %q row, got %d", key, rows[key])
		}
		if !hdr[key] {
			t.Errorf("%q must be hdr:true or it ships undiscoverable", key)
		}
	}
}

func TestErrorPaneRendersOnlyStderr(t *testing.T) {
	m := settledTestModel(3)
	m.vp = viewport{width: 100, height: 40}
	m.errOpen = true
	m.appendLogLine("h1", "installing packages", false)
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)

	v := m.View()
	if !strings.Contains(v, "WARNING: apt-get update failed") {
		t.Fatal("the error pane must show the stderr line")
	}
	// The log pane keeps EVERYTHING: closing the error pane must not lose it.
	m.errOpen = false
	if !strings.Contains(m.View(), "WARNING: apt-get update failed") {
		t.Fatal("stderr must remain visible in the log pane")
	}
}

func TestErrorPaneOpenButEmptyCollapses(t *testing.T) {
	m := settledTestModel(3)
	m.vp = viewport{width: 100, height: 40}
	m.errOpen = true
	rowsWithout := m.listHeight()
	m.appendLogLine("h1", "boom", true)
	if m.listHeight() >= rowsWithout {
		t.Fatal("once stderr flows the error pane claims height and the list shrinks")
	}
}
