package cmd

import (
	"errors"
	"strings"
	"testing"
)

// Owner-reported: after an update finished, the dots stayed green/red for the
// rest of the session. The outcome outranked the selection forever, so
// toggling a host with space changed m.selected but nothing on screen — the
// selection had become invisible. The dot keeps its outcome after a wave, but
// `r` hands every finished host's dot back to the selection, and taking a
// host out of the selection always clears its outcome dot.

func markOf(m tuiModel, alias string) markKind { return m.markState(m.indexOf(alias)) }

// finished runs a/b/c through a wave: a and b selected and updated (a ok,
// b failed), c updated from the cursor with nothing selected (ok).
func finished(t *testing.T) tuiModel {
	t.Helper()
	m := testModel("a", "b", "c")
	m.selected["a"], m.selected["b"] = true, true
	m.finishUpdate("a", "", nil)
	m.finishUpdate("b", "", errors.New("boom"))
	m.finishUpdate("c", "", nil)
	if markOf(m, "a") != markOK || markOf(m, "b") != markFail || markOf(m, "c") != markOK {
		t.Fatalf("a finished wave shows its outcome on the dot: a=%v b=%v c=%v",
			markOf(m, "a"), markOf(m, "b"), markOf(m, "c"))
	}
	return m
}

func TestDeselectClearsTheOutcomeDot(t *testing.T) {
	m := finished(t)
	m.cursor = "a"
	m, _ = send(m, " ") // deselect a
	if got := markOf(m, "a"); got != markNone {
		t.Fatalf("deselecting a finished host must clear its dot, got %v", got)
	}
	if !strings.Contains(m.updateCell("a"), "ok") {
		t.Fatalf("the UPDATE column keeps the outcome text, got %q", m.updateCell("a"))
	}
	m, _ = send(m, " ") // select it again
	if got := markOf(m, "a"); got != markSel {
		t.Fatalf("once cleared, selecting shows the selection dot, got %v", got)
	}

	m.cursor = "b"
	m, _ = send(m, " ") // deselect the FAILED host too — "regardless of what's showing"
	if got := markOf(m, "b"); got != markNone {
		t.Fatalf("deselecting a failed host must clear its dot, got %v", got)
	}
	if !strings.Contains(m.updateCell("b"), "FAIL") {
		t.Fatalf("the failure cause stays in the UPDATE column, got %q", m.updateCell("b"))
	}
}

func TestRefreshHandsTheDotsBackToTheSelection(t *testing.T) {
	m := finished(t)
	m, _ = send(m, "r")
	if markOf(m, "a") != markSel || markOf(m, "b") != markSel {
		t.Fatalf("after r the selected hosts show the selection dot: a=%v b=%v", markOf(m, "a"), markOf(m, "b"))
	}
	if got := markOf(m, "c"); got != markNone {
		t.Fatalf("after r an unselected host shows no dot, got %v", got)
	}
	if !strings.Contains(m.updateCell("b"), "FAIL") {
		t.Fatalf("r keeps the UPDATE column's outcome, got %q", m.updateCell("b"))
	}

	// From here selection and deselection are visible again.
	m.cursor = "c"
	m, _ = send(m, " ")
	if got := markOf(m, "c"); got != markSel {
		t.Fatalf("selecting after r shows the selection dot, got %v", got)
	}
	m, _ = send(m, " ")
	if got := markOf(m, "c"); got != markNone {
		t.Fatalf("deselecting after r clears the dot, got %v", got)
	}
}

// Owner follow-up: a green/red dot is a terminal state and the run stays in
// history, so selecting such a host clears it too — no `r` needed first.
func TestSelectingAFinishedHostShowsTheSelection(t *testing.T) {
	m := finished(t)
	m.cursor = "c" // unselected, green from a cursor-only update
	m, _ = send(m, " ")
	if got := markOf(m, "c"); got != markSel {
		t.Fatalf("selecting a finished host shows the selection dot, got %v", got)
	}

	// Visual range select (v, move, space) clears every host it adds.
	m = finished(t)
	m.selected = map[string]bool{}
	m.cursor = "a"
	m, _ = send(m, "v", "j", "j", " ")
	for _, h := range []string{"a", "b", "c"} {
		if got := markOf(m, h); got != markSel {
			t.Fatalf("visual select must clear %s's outcome dot, got %v", h, got)
		}
	}

	// Select-all (a) with part of the rows unselected completes the selection.
	m = finished(t)
	m, _ = send(m, "a")
	if got := markOf(m, "c"); got != markSel {
		t.Fatalf("select-all must clear c's outcome dot, got %v", got)
	}
}

// Selecting a host mid-update changes nothing about its coming outcome.
func TestSelectingAnInFlightHostKeepsItsComingOutcome(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updRunning}
	m.cursor = "a"
	m, _ = send(m, " ")
	if got := markOf(m, "a"); got != markSel {
		t.Fatalf("a selected in-flight host shows the selection dot, got %v", got)
	}
	m.finishUpdate("a", "", nil)
	if got := markOf(m, "a"); got != markOK {
		t.Fatalf("its outcome still shows when it lands, got %v", got)
	}
}

// Owner follow-up: deselecting a host mid-update must not make its update
// status go away. Deselect clears only a FINISHED outcome from the dot; the
// UPDATE column keeps showing queued / precheck / updating, the engine keeps
// owning the host, and the outcome still lands on the dot.
func TestDeselectDuringAnUpdateKeepsTheUpdateStatus(t *testing.T) {
	phases := map[string]updPhase{"q": updQueued, "p": updPrecheck, "u": updRunning}
	cells := map[string]string{"q": "queued", "p": "precheck", "u": "updating"}
	for _, how := range []string{"space", "a", "esc"} {
		m := testModel("q", "p", "u")
		for h, ph := range phases {
			m.selected[h] = true
			m.updating[h] = updState{phase: ph}
		}
		m.bgQueue = []string{"q"}
		switch how {
		case "space":
			for h := range phases {
				m.cursor = h
				m, _ = send(m, " ")
			}
		default:
			m, _ = send(m, how)
		}
		for h, ph := range phases {
			if len(m.selected) != 0 {
				t.Fatalf("%s: the hosts must be deselected, selection %v", how, m.selected)
			}
			if got := m.updating[h].phase; got != ph {
				t.Fatalf("%s: deselecting %s must not touch its update, phase %v -> %v", how, h, ph, got)
			}
			if !m.inFlight(h) {
				t.Fatalf("%s: %s must still be owned by the update engine", how, h)
			}
			if got := m.updateCell(h); !strings.Contains(got, cells[h]) {
				t.Fatalf("%s: %s's UPDATE column must still say %q, got %q", how, h, cells[h], got)
			}
		}
		if len(m.bgQueue) != 1 || m.bgQueue[0] != "q" {
			t.Fatalf("%s: deselecting must not drop a queued host from the queue, got %v", how, m.bgQueue)
		}
		m.finishUpdate("u", "", nil)
		m.finishUpdate("p", "", errors.New("boom"))
		if markOf(m, "u") != markOK || markOf(m, "p") != markFail {
			t.Fatalf("%s: outcomes that land after a deselect still show: u=%v p=%v", how, markOf(m, "u"), markOf(m, "p"))
		}
	}
}

// r is an acknowledgment of what has finished — not of what is still running.
func TestRefreshLeavesInFlightAndLaterOutcomesAlone(t *testing.T) {
	m := finished(t)
	m.updating["a"] = updState{phase: updRunning} // a second wave is running on a
	m, _ = send(m, "r")
	if got := m.updating["a"].phase; got != updRunning {
		t.Fatalf("r must not touch a host the engine owns, phase %v", got)
	}
	m.finishUpdate("a", "", errors.New("again"))
	if got := markOf(m, "a"); got != markFail {
		t.Fatalf("an outcome that lands after r shows on the dot again, got %v", got)
	}

	// Same for a deselected host that is updated again.
	m.cursor = "c"
	m.finishUpdate("c", "", nil)
	if got := markOf(m, "c"); got != markOK {
		t.Fatalf("a new outcome replaces a cleared one, got %v", got)
	}
}

// Every way a host leaves the selection clears its outcome dot, not just space.
func TestBulkDeselectClearsOutcomeDots(t *testing.T) {
	m := finished(t)
	m.selected["c"] = true
	m, _ = send(m, "a") // everything selected -> a clears the selection
	for _, h := range []string{"a", "b", "c"} {
		if got := markOf(m, h); got != markNone {
			t.Fatalf("select-all toggling off must clear %s's dot, got %v", h, got)
		}
	}

	m = finished(t)
	m, _ = send(m, "esc") // esc clears the selection
	if markOf(m, "a") != markNone || markOf(m, "b") != markNone {
		t.Fatalf("esc clearing the selection must clear the dots: a=%v b=%v", markOf(m, "a"), markOf(m, "b"))
	}
	if got := markOf(m, "c"); got != markOK {
		t.Fatalf("c was never selected, so deselection did not touch it, got %v", got)
	}
}
