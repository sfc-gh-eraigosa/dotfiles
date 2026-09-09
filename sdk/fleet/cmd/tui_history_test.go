package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errTestScan = errors.New("scan boom")

func histCapture(t *testing.T, dir, stamp, host, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, stamp+"__"+host+".log"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const histRun = `# fleet update — host=x started=y
03:47:14 === step dotfiles.sync (sync) ===
03:47:16 Already up to date.
# 2026-09-09T03:50:36Z finished
`

// TestHistoryRunsScopeToTheGivenHosts pins that history follows the SELECTION,
// the same rule `u` and `w` already use via updateTargets(): an operator who
// has selected three hosts means those three, and one who has selected none
// means the host under the cursor. Loading the whole fleet regardless would
// discard the scoping the operator just did by hand.
func TestHistoryRunsScopeToTheGivenHosts(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T040000Z", "beta", histRun)
	histCapture(t, dir, "20260909T050000Z", "gamma", histRun)

	runs, err := historyRuns(dir, []string{"alpha", "gamma"})
	if err != nil {
		t.Fatalf("historyRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want alpha+gamma only: %+v", len(runs), runs)
	}
	// newest first, so gamma (05:00) leads alpha (03:00)
	if runs[0].Host != "gamma" || runs[1].Host != "alpha" {
		t.Errorf("want gamma then alpha (newest first), got %s then %s", runs[0].Host, runs[1].Host)
	}
}

// TestHistoryRunsWithNoHostsIsTheWholeFleet pins the wider scope: an empty
// host list means "everything", which is what an empty selection with no
// cursor (an empty fleet, or a search that matched nothing) should show
// rather than an error.
func TestHistoryRunsWithNoHostsIsTheWholeFleet(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T040000Z", "beta", histRun)

	runs, err := historyRuns(dir, nil)
	if err != nil {
		t.Fatalf("historyRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want both: %+v", len(runs), runs)
	}
}

// TestHistoryRunsCarriesTheSummary pins that the list has what it needs to be
// triaged without opening anything: whether the run finished, and its warning
// count. Those require the file to be read, which is why the list holds
// Summary rather than the filename-only Run.
func TestHistoryRunsCarriesTheSummary(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)

	runs, err := historyRuns(dir, []string{"alpha"})
	if err != nil {
		t.Fatalf("historyRuns: %v", err)
	}
	if !runs[0].Finished {
		t.Errorf("a capture with a footer is finished: %+v", runs[0])
	}
	if runs[0].Path == "" || !strings.HasSuffix(runs[0].Path, "__alpha.log") {
		t.Errorf("the run must carry its path so it can be opened: %+v", runs[0])
	}
}

// TestHistoryRunsWithNoLogDirIsEmptyNotAnError pins the fresh-machine case:
// a dashboard on a host that has never run an update opens history to an
// empty list and a sentence, never a failure — the same rule fleet applies
// to a missing ~/.ssh/config.
func TestHistoryRunsWithNoLogDirIsEmptyNotAnError(t *testing.T) {
	runs, err := historyRuns(filepath.Join(t.TempDir(), "nope"), []string{"alpha"})
	if err != nil {
		t.Fatalf("a missing log dir must not be an error: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("want no runs, got %+v", runs)
	}
}

// TestHistoryKeyScopeFollowsTheSelection pins that H obeys the same rule as
// `u` and `w`: the selection if there is one, otherwise the cursor host. An
// operator who has just selected three hosts and presses H means those
// three; widening to the whole fleet would silently discard the scoping they
// did by hand.
func TestHistoryKeyScopeFollowsTheSelection(t *testing.T) {
	m := testModel("alpha", "beta", "gamma")

	// no selection: the cursor host alone
	m2, cmd := send(m, "H")
	if !m2.histOn {
		t.Fatal("H must enter history")
	}
	if cmd == nil {
		t.Error("H must return a Cmd to load the runs — I/O never happens in Update")
	}
	if len(m2.histScope) != 1 || m2.histScope[0] != m.cursor {
		t.Errorf("scope = %v, want the cursor host %q", m2.histScope, m.cursor)
	}

	// with a selection: exactly the selected hosts
	m3, _ := send(m, "space", "j", "space")
	sel := m3.updateTargets()
	m4, _ := send(m3, "H")
	if len(m4.histScope) != len(sel) {
		t.Errorf("scope = %v, want the selection %v", m4.histScope, sel)
	}
}

// TestHistoryKeyTogglesBackToTheDashboard pins that H is a toggle and that
// leaving restores the dashboard exactly — history is VIEW state, so the
// host cursor it was scoped from must survive the round trip untouched.
func TestHistoryKeyTogglesBackToTheDashboard(t *testing.T) {
	m := testModel("alpha", "beta", "gamma")
	before := m.cursor

	m2, _ := send(m, "H")
	if !m2.histOn {
		t.Fatal("H must enter history")
	}
	m3, _ := send(m2, "H")
	if m3.histOn {
		t.Error("a second H must return to the dashboard")
	}
	if m3.cursor != before {
		t.Errorf("host cursor = %q, want it preserved as %q across the round trip", m3.cursor, before)
	}
}

// TestHistoryLoadedCursorsTheNewestRun pins where the cursor lands. The run
// list is newest-first, and "what happened last" is the question someone
// opening history is nearly always asking.
func TestHistoryLoadedCursorsTheNewestRun(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T050000Z", "alpha", histRun)

	runs, err := historyRuns(dir, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}

	m, _ := send(testModel("alpha"), "H")
	mm, _ := m.Update(historyLoadedMsg{runs: runs})
	m2 := mm.(tuiModel)

	if len(m2.histRuns) != 2 {
		t.Fatalf("want both runs loaded, got %d", len(m2.histRuns))
	}
	if m2.histCursor != runs[0].Path {
		t.Errorf("cursor = %q, want the newest run %q", m2.histCursor, runs[0].Path)
	}
}

// TestHistoryLoadFailureIsSaidNotSwallowed pins that a scan error reaches the
// operator. An empty list that silently means "I could not read the
// directory" is the same class of lie as calling an unobserved run clean.
func TestHistoryLoadFailureIsSaidNotSwallowed(t *testing.T) {
	m, _ := send(testModel("alpha"), "H")
	mm, _ := m.Update(historyLoadedMsg{err: errTestScan})
	m2 := mm.(tuiModel)

	if m2.status == "" || !strings.Contains(m2.status, "history") {
		t.Errorf("a failed history load must be reported in the status line, got %q", m2.status)
	}
}

// histModelWith returns a model already in history with runs loaded, so the
// motion tests exercise the real key path rather than poking state.
func histModelWith(t *testing.T, n int) tuiModel {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		histCapture(t, dir, fmt.Sprintf("2026090%dT030000Z", i+1), "alpha", histRun)
	}
	runs, err := historyRuns(dir, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := send(testModel("alpha"), "H")
	mm, _ := m.Update(historyLoadedMsg{runs: runs})
	return mm.(tuiModel)
}

// TestHistoryMotionUsesTheSameKeysAsTheHostList pins the payoff of history
// being view state rather than a mode: j/k/G/gg and the half-page motions
// move the RUN cursor with no new bindings and no duplicated routing. If
// this needed its own key table, history should have been a mode.
func TestHistoryMotionUsesTheSameKeysAsTheHostList(t *testing.T) {
	m := histModelWith(t, 4)
	first := m.histCursor

	m2, _ := send(m, "j")
	if m2.histCursor == first {
		t.Error("j must move the run cursor")
	}
	m3, _ := send(m2, "k")
	if m3.histCursor != first {
		t.Errorf("k must move back to %q, got %q", first, m3.histCursor)
	}

	m4, _ := send(m, "G")
	if m4.histCursor != m.histRuns[len(m.histRuns)-1].Path {
		t.Error("G must jump to the oldest run")
	}
	m5, _ := send(m4, "g", "g")
	if m5.histCursor != first {
		t.Error("gg must jump back to the newest run")
	}
}

// TestHistoryMotionLeavesTheHostCursorAlone pins the separation: the two
// cursors are independent, so leaving history puts the operator back exactly
// where they were in the host list.
func TestHistoryMotionLeavesTheHostCursorAlone(t *testing.T) {
	m := histModelWith(t, 3)
	host := m.cursor

	m2, _ := send(m, "j", "j", "G")
	if m2.cursor != host {
		t.Errorf("host cursor moved to %q while navigating history; want %q", m2.cursor, host)
	}
}

// TestHistoryMotionClampsAtBothEnds pins that motion cannot run off the list
// — the same clamping moveTo already gives the host list.
func TestHistoryMotionClampsAtBothEnds(t *testing.T) {
	m := histModelWith(t, 3)

	m2, _ := send(m, "k", "k", "k", "k")
	if m2.histCursor != m.histRuns[0].Path {
		t.Error("k past the top must clamp to the newest run")
	}
	m3, _ := send(m, "j", "j", "j", "j", "j")
	if m3.histCursor != m.histRuns[len(m.histRuns)-1].Path {
		t.Error("j past the bottom must clamp to the oldest run")
	}
}
