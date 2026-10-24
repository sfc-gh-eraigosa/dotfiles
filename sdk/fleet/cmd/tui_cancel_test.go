package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

// running puts alias in flight the way pump + streamStartedMsg do, with a
// cancel func that records its call.
func running(t *testing.T, m tuiModel, alias string) (tuiModel, *bool) {
	t.Helper()
	called := new(bool)
	m.updating[alias] = updState{phase: updRunning}
	m.running++
	mm, _ := m.Update(streamStartedMsg{alias: alias, st: stream{
		lines: make(chan outLine), done: make(chan error, 1),
	}, cancel: func() { *called = true }})
	return mm.(tuiModel), called
}

func ctrlC(m tuiModel) (tuiModel, tea.Cmd) {
	mm, c := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	return mm.(tuiModel), c
}

func isQuit(c tea.Cmd) bool {
	if c == nil {
		return false
	}
	_, ok := c().(tea.QuitMsg)
	return ok
}

func TestCtrlCArmsThenCancelsTheRunningUpdate(t *testing.T) {
	m, called := running(t, testModel("a", "b"), "a")
	m.cursor = "a"

	m, c := ctrlC(m)
	if *called || isQuit(c) {
		t.Fatal("the first ctrl+c must only arm: nothing cancelled, nothing quit")
	}
	if !strings.Contains(m.status, "ctrl+c again") || !strings.Contains(m.status, "a") {
		t.Fatalf("the armed status must name the host and the confirming key: %q", m.status)
	}
	m, c = ctrlC(m)
	if !*called {
		t.Fatal("the second ctrl+c must cancel the update")
	}
	if isQuit(c) {
		t.Fatal("cancelling an update must not quit the dashboard")
	}
	if !strings.Contains(m.status, "cancelling") {
		t.Fatalf("status must say the cancel is under way: %q", m.status)
	}
}

func TestAnyOtherKeyDisarmsCtrlC(t *testing.T) {
	m, called := running(t, testModel("a", "b"), "a")
	m.cursor = "a"
	m, _ = ctrlC(m)
	m, _ = send(m, "j")
	m, _ = ctrlC(m)
	if *called {
		t.Fatal("a key between the two presses must disarm the cancel")
	}
}

func TestCtrlCCancelsTheSelectionOnly(t *testing.T) {
	m, aCalled := running(t, testModel("a", "b"), "a")
	m, bCalled := running(t, m, "b")
	m.cursor = "a"
	m.selected["b"] = true
	m, _ = ctrlC(m)
	_, _ = ctrlC(m)
	if !*bCalled || *aCalled {
		t.Fatalf("ctrl+c must cancel the selection, like u updates it: a=%v b=%v", *aCalled, *bCalled)
	}
}

func TestCtrlCOnAnIdleCursorCancelsEverythingInFlight(t *testing.T) {
	m, aCalled := running(t, testModel("a", "b", "c"), "a")
	m, bCalled := running(t, m, "b")
	m.cursor = "c" // idle, nothing selected
	m, _ = ctrlC(m)
	_, _ = ctrlC(m)
	if !*aCalled || !*bCalled {
		t.Fatalf("with no in-flight target in scope, ctrl+c cancels every update: a=%v b=%v", *aCalled, *bCalled)
	}
}

func TestCancelDropsQueuedHosts(t *testing.T) {
	m := testModel("a", "b")
	m.updating["a"] = updState{phase: updQueued}
	m.updating["b"] = updState{phase: updQueued}
	m.bgQueue = []string{"a"}
	m.iaQueue = []string{"b"}
	m.selected["a"], m.selected["b"] = true, true
	m, _ = ctrlC(m)
	m, _ = ctrlC(m)
	if len(m.bgQueue)+len(m.iaQueue) != 0 {
		t.Fatalf("cancelled hosts must leave the queues: bg=%v ia=%v", m.bgQueue, m.iaQueue)
	}
	for _, a := range []string{"a", "b"} {
		if m.updating[a].phase != updCancelled {
			t.Fatalf("%s phase = %v, want cancelled", a, m.updating[a].phase)
		}
	}
	if m.busy() {
		t.Fatal("nothing is left in flight after the queue is cancelled")
	}
}

func TestCancelDuringPrecheckDropsTheHostWhenItsVerdictArrives(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updPrecheck}
	m.cursor = "a"
	m, _ = ctrlC(m)
	m, _ = ctrlC(m)
	mm, _ := m.Update(precheckMsg{alias: "a"})
	got := mm.(tuiModel)
	if got.updating["a"].phase != updCancelled || got.running != 0 || len(got.bgQueue) != 0 {
		t.Fatalf("a host cancelled in precheck must never start: phase=%v running=%d queue=%v",
			got.updating["a"].phase, got.running, got.bgQueue)
	}
}

func TestCancelBeforeTheStreamStartsCancelsItOnArrival(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updRunning} // pump issued beginStream; no cancel func yet
	m.running = 1
	m.cursor = "a"
	m, _ = ctrlC(m)
	m, _ = ctrlC(m)
	called := false
	_, _ = m.Update(streamStartedMsg{alias: "a", st: stream{lines: make(chan outLine), done: make(chan error, 1)},
		cancel: func() { called = true }})
	if !called {
		t.Fatal("a cancel requested before the stream started must fire as soon as it starts")
	}
}

func TestACancelledRunIsShownAsCancelledNotFailed(t *testing.T) {
	m, _ := running(t, testModel("a"), "a")
	rep := updexec.HostReport{Results: []updexec.Result{{Step: "dotfiles.install", Status: updexec.Cancelled, Reason: "cancelled"}}}
	mm, _ := m.Update(bgUpdateDoneMsg{alias: "a", err: bgDoneErr(rep)})
	got := mm.(tuiModel)
	if got.updating["a"].phase != updCancelled {
		t.Fatalf("phase = %v, want cancelled", got.updating["a"].phase)
	}
	if got.running != 0 {
		t.Fatalf("the slot must be released: running=%d", got.running)
	}
	if v := got.View(); !strings.Contains(v, "cancelled") || strings.Contains(v, "FAIL") {
		t.Fatalf("the row must read cancelled, not FAIL:\n%s", v)
	}
}

func TestBgDoneErrMarksACancelledReport(t *testing.T) {
	rep := updexec.HostReport{Results: []updexec.Result{{Step: "s", Status: updexec.Cancelled, Reason: "cancelled"}}}
	if err := bgDoneErr(rep); !isCancelled(err) {
		t.Fatalf("bgDoneErr(cancelled) = %v, want errCancelled", err)
	}
}

func TestCtrlCWithNothingInFlightQuits(t *testing.T) {
	_, c := ctrlC(testModel("a"))
	if !isQuit(c) {
		t.Fatal("with nothing to cancel, ctrl+c still quits")
	}
}

func TestQuitCancelsEveryUpdateAndExitsOnceTheyStop(t *testing.T) {
	m, aCalled := running(t, testModel("a", "b"), "a")
	m.updating["b"] = updState{phase: updQueued}
	m.bgQueue = []string{"b"}

	m, c := send(m, "q")
	if isQuit(c) || *aCalled {
		t.Fatal("the first q only warns")
	}
	m, c = send(m, "q")
	if !*aCalled {
		t.Fatal("the second q must cancel the running update")
	}
	if len(m.bgQueue) != 0 || m.updating["b"].phase != updCancelled {
		t.Fatal("the second q must drop the queued host")
	}
	if isQuit(c) {
		t.Fatal("quit must wait for the cancelled update to stop")
	}
	mm, c := m.Update(bgUpdateDoneMsg{alias: "a", err: errCancelled})
	if !isQuit(c) {
		t.Fatalf("the dashboard must exit once the last update stopped (status %q)", mm.(tuiModel).status)
	}
}

func TestThirdQQuitsWithoutWaiting(t *testing.T) {
	m, _ := running(t, testModel("a"), "a")
	m, _ = send(m, "q", "q")
	_, c := send(m, "q")
	if !isQuit(c) {
		t.Fatal("a third q must quit immediately")
	}
}

// fleet's cancel key is the shared sdk one (keymap.Interrupt, GUIDE.md §3):
// fleet routes keys by hand until it moves onto libs/tui, so this pins the
// two together — a change to the shared key fails here, not in a user's hands.
func TestCancelKeyIsTheSharedInterruptBinding(t *testing.T) {
	documented := map[string]bool{}
	for _, k := range keyHelp {
		documented[k.keys] = true
	}
	for _, key := range keymap.Interrupt.Keys {
		if !documented[key] {
			t.Errorf("keymap.Interrupt binds %q but fleet's keyHelp does not document it", key)
		}
	}
	if keymap.Interrupt.Action != keymap.Cancel {
		t.Fatalf("keymap.Interrupt is %q, want %q", keymap.Interrupt.Action, keymap.Cancel)
	}

	m, called := running(t, testModel("a"), "a")
	m, _ = ctrlC(m)
	m, _ = ctrlC(m)
	if !*called {
		t.Error("ctrl+c twice must cancel the running update, as keymap.Interrupt says")
	}
}

// A first q answered by the updates ending on their own is spent: the next
// update starts the warning over instead of one q cancelling it.
func TestAnUnansweredQuitWarningLapsesWhenUpdatesEnd(t *testing.T) {
	m, _ := running(t, testModel("a"), "a")
	m, _ = send(m, "q")
	if !m.quitReq {
		t.Fatal("the first q must warn")
	}
	mm, _ := m.Update(bgUpdateDoneMsg{alias: "a"})
	m = mm.(tuiModel)
	if m.quitReq {
		t.Fatal("the warning must lapse once nothing is running")
	}
	m, called := running(t, m, "a")
	m, c := send(m, "q")
	if *called || isQuit(c) || m.quitting {
		t.Fatal("a q after the warning lapsed must warn again, not cancel or quit")
	}
}
