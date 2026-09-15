package cmd

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Pins the repaint rule documented on layoutSig in tui_model.go. Owner-reported
// (Termius on an iPad): after a second update and toggling the stderr pane,
// the host list showed duplicate rows — two older `precheck` rows above the
// four live ones — and two `logs` headers. The model had no duplicates; what
// duplicates rows is bubbletea's line-diff renderer meeting a terminal whose
// usable rows differ from what it reported. A layout change is when that
// becomes visible, so it is when Update batches tea.ClearScreen. Ordinary keys
// and later streamed lines must NOT clear the screen — that would flicker on
// every line of output.

// cmdMsgs runs cmd and returns the messages it produces, flattening batches.
// A command that does not answer promptly is a stream reader parked on a
// channel — by construction not a repaint — and contributes nothing.
func cmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			return []tea.Msg{msg}
		}
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, cmdMsgs(c)...)
		}
		return msgs
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// isClearScreen reports whether msg is bubbletea's ClearScreen. clearScreenMsg
// is an empty struct, so interface equality against the constructor is exact.
func isClearScreen(msg tea.Msg) bool { return msg == tea.ClearScreen() }

// hasClearScreen reports whether cmd (or anything it batches) repaints.
func hasClearScreen(cmd tea.Cmd) bool {
	for _, msg := range cmdMsgs(cmd) {
		if isClearScreen(msg) {
			return true
		}
	}
	return false
}

// repaintOnly reports whether cmd does nothing but repaint: nil, or a
// ClearScreen (possibly batched with nothing else). Tests that used a nil cmd
// as "no update was started" use this, since a dialog opening or closing is a
// layout change and now carries the repaint.
func repaintOnly(cmd tea.Cmd) bool {
	if cmd == nil {
		return true
	}
	msgs := cmdMsgs(cmd)
	if len(msgs) == 0 {
		return false // parked or timed out: something other than a repaint
	}
	for _, msg := range msgs {
		if !isClearScreen(msg) {
			return false
		}
	}
	return true
}

// streaming gives alias a drained stream so the reader update re-issues after
// a logLineMsg returns logEOFMsg at once instead of parking on a nil channel.
func streaming(m tuiModel, alias string) tuiModel {
	ch := make(chan outLine)
	close(ch)
	m.streams[alias] = stream{lines: ch}
	return m
}

func TestLayoutChangesRepaintTheWholeScreen(t *testing.T) {
	m := testModel("a", "b")
	m.vp = viewport{height: 40, width: 120}
	for _, tc := range []struct {
		keys []string
		what string
	}{
		{[]string{"e"}, "stderr pane opened"},
		{[]string{"e", "e"}, "stderr pane closed again"},
		{[]string{"l"}, "log pane hidden"},
		{[]string{"h"}, "host list hidden"},
		{[]string{"u"}, "answers form opened"},
		{[]string{"u", "esc"}, "answers form closed"},
		{[]string{"?"}, "help opened"},
		{[]string{"?", "j"}, "help closed"},
	} {
		if _, cmd := send(m, tc.keys...); !hasClearScreen(cmd) {
			t.Fatalf("%s: the frame must be repainted from scratch (keys %v)", tc.what, tc.keys)
		}
	}
}

// The first line of output is the biggest layout change of a session: the
// host list drops from full height to a fifth and the log pane grows from a
// one-line hint to the whole budget — with no pane toggled. The heights key on
// logActive/errActive (open AND non-empty), so the signature must too.
func TestFirstStreamedLineRepaints(t *testing.T) {
	m := streaming(testModel("a", "b"), "a")
	m.vp = viewport{height: 40, width: 120}

	listBefore, logBefore := m.listHeight(), m.logHeight()
	next, cmd := m.Update(logLineMsg{alias: "a", line: "Installing sops..."})
	m2 := next.(tuiModel)
	if m2.listHeight() == listBefore || m2.logHeight() == logBefore {
		t.Fatalf("premise: the first line must re-layout the frame, list %d->%d log %d->%d",
			listBefore, m2.listHeight(), logBefore, m2.logHeight())
	}
	if !hasClearScreen(cmd) {
		t.Fatal("the first streamed line re-lays out the frame and must repaint it")
	}

	// The first stderr line with the error pane open halves the log pane.
	m2.errOpen = true
	errBefore := m2.errHeight()
	next, cmd = m2.Update(logLineMsg{alias: "a", line: "WARNING: x", stderr: true})
	m3 := next.(tuiModel)
	if m3.errHeight() == errBefore {
		t.Fatalf("premise: the first stderr line must open the error pane, err %d->%d", errBefore, m3.errHeight())
	}
	if !hasClearScreen(cmd) {
		t.Fatal("the first stderr line re-lays out the frame and must repaint it")
	}
}

func TestOrdinaryInputDoesNotRepaintTheWholeScreen(t *testing.T) {
	// Output already flowing on both streams: the layout has settled, and
	// every later line lands inside a pane whose height does not move.
	m := streaming(testModel("a", "b"), "a")
	m.vp = viewport{height: 40, width: 120}
	m.errOpen = true
	for _, msg := range []tea.Msg{
		logLineMsg{alias: "a", line: "Installing sops..."},
		logLineMsg{alias: "a", line: "WARNING: x", stderr: true},
	} {
		next, _ := m.Update(msg)
		m = next.(tuiModel)
	}

	for _, tc := range []struct {
		msg  tea.Msg
		what string
	}{
		{key("j"), "cursor move"},
		{key(" "), "selection toggle"},
		{key("v"), "visual anchor"},
		{logLineMsg{alias: "a", line: "Installing age..."}, "a later streamed line"},
		{logLineMsg{alias: "a", line: "WARNING: y", stderr: true}, "a later streamed stderr line"},
		{hostRowMsg{row: Row{Alias: "a", Class: "up-to-date"}}, "a row arriving"},
		{tea.WindowSizeMsg{Width: 100, Height: 30}, "a resize (bubbletea repaints that itself)"},
	} {
		if _, cmd := m.Update(tc.msg); hasClearScreen(cmd) {
			t.Fatalf("%s must not clear the screen — that flickers on every event", tc.what)
		}
	}

	// The search prompt reuses the status row, so entering and leaving search
	// changes no geometry; a clear there would only flicker.
	for _, keys := range [][]string{{"/"}, {"/", "a", "enter"}, {"/", "esc"}} {
		if _, cmd := send(m, keys...); hasClearScreen(cmd) {
			t.Fatalf("search (keys %v) is not a layout change", keys)
		}
	}

	// A refused toggle changes nothing, so it repaints nothing: with only the
	// stderr pane left open, `e` is refused ("at least one view must stay open").
	only := m
	only.hostOpen, only.logOpen, only.errOpen = false, false, true
	if _, cmd := only.Update(key("e")); hasClearScreen(cmd) {
		t.Fatal("a refused pane toggle is not a layout change")
	}
}

// The repaint rides alongside whatever the key already returned; it must
// never replace it (a history toggle also has a load to issue).
func TestRepaintIsBatchedWithTheKeysOwnCommand(t *testing.T) {
	m := testModel("a")
	m.vp = viewport{height: 40, width: 120}
	m.logDir = t.TempDir()
	_, cmd := m.Update(key("H"))
	msgs := cmdMsgs(cmd)
	if !hasClearScreen(cmd) {
		t.Fatalf("opening history is a layout change, got %v", msgs)
	}
	loaded := false
	for _, msg := range msgs {
		if _, ok := msg.(historyLoadedMsg); ok {
			loaded = true
		}
	}
	if !loaded {
		t.Fatalf("the history load must still be issued next to the repaint, got %v", msgs)
	}
}
