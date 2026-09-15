package cmd

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Owner-reported (Termius on an iPad): after a second update and toggling the
// stderr pane, the host list showed duplicate rows — two older `precheck`
// rows above the four live ones — and two `logs` headers. The model had no
// duplicates: every frame in that sequence is exactly the terminal's height
// and no line is wider than it (checked at five sizes). What duplicates rows
// is bubbletea's renderer meeting a terminal whose usable rows differ from
// what it reported: a full-height frame scrolls the alt screen by one, and
// from then on bubbletea repaints only lines whose text changed, so unchanged
// lines keep their scrolled-up content. A layout change (a pane toggled, a
// dialog opened) is when that becomes visible, so it is also when the frame
// is repainted from scratch: Update batches tea.ClearScreen whenever the
// layout signature changed. Ordinary keys and streamed lines must NOT clear
// the screen — that would flicker on every line of output.

// hasClearScreen runs cmd (and whatever it batches) and reports whether
// bubbletea's ClearScreen message is among the results. A command that does
// not answer promptly is a stream reader parked on a channel (the reissued
// read after a logLineMsg) — by construction not a repaint.
func hasClearScreen(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if hasClearScreen(c) {
					return true
				}
			}
			return false
		}
		return fmt.Sprintf("%T", msg) == "tea.clearScreenMsg"
	case <-time.After(300 * time.Millisecond):
		return false
	}
}

// repaintOnly reports whether cmd does nothing but repaint: nil, or a
// ClearScreen (possibly batched with nothing else). Tests that used a nil cmd
// as "no update was started" use this, since a dialog opening or closing is a
// layout change and now carries the repaint.
func repaintOnly(cmd tea.Cmd) bool {
	if cmd == nil {
		return true
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if !repaintOnly(c) {
					return false
				}
			}
			return true
		}
		return fmt.Sprintf("%T", msg) == "tea.clearScreenMsg"
	case <-time.After(300 * time.Millisecond):
		return false
	}
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
		var mm tea.Model = m
		var cmd tea.Cmd
		for _, k := range tc.keys {
			mm, cmd = mm.Update(key(k))
		}
		if !hasClearScreen(cmd) {
			t.Fatalf("%s: the frame must be repainted from scratch (keys %v)", tc.what, tc.keys)
		}
	}
}

func TestOrdinaryInputDoesNotRepaintTheWholeScreen(t *testing.T) {
	m := testModel("a", "b")
	m.vp = viewport{height: 40, width: 120}
	for _, tc := range []struct {
		msg  tea.Msg
		what string
	}{
		{key("j"), "cursor move"},
		{key(" "), "selection toggle"},
		{key("v"), "visual anchor"},
		{logLineMsg{alias: "a", line: "Installing sops..."}, "a streamed line"},
		{logLineMsg{alias: "a", line: "WARNING: x", stderr: true}, "a streamed stderr line"},
		{hostRowMsg{row: Row{Alias: "a", Class: "up-to-date"}}, "a row arriving"},
		{tea.WindowSizeMsg{Width: 100, Height: 30}, "a resize (bubbletea repaints that itself)"},
	} {
		_, cmd := m.Update(tc.msg)
		if hasClearScreen(cmd) {
			t.Fatalf("%s must not clear the screen — that flickers on every event", tc.what)
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
	if !hasClearScreen(cmd) {
		t.Fatal("opening history is a layout change")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("the history load must still be issued next to the repaint, got %T", cmd())
	}
}
