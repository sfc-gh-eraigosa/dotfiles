package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

// TestHistoryPanelListsTheRuns pins what the run list shows: enough to triage
// without opening anything — when it ran, whether it finished, and its
// warning count — which is why the list holds Summary rather than Run.
func TestHistoryPanelListsTheRuns(t *testing.T) {
	m := histModelWith(t, 3)
	m.vp.width, m.vp.height = 120, 40

	out := m.histPanel()
	for _, want := range []string{"WHEN", "RESULT", "WARN"} {
		if !strings.Contains(out, want) {
			t.Errorf("run list must carry a %q column:\n%s", want, out)
		}
	}
	if strings.Count(out, "finished") < 3 {
		t.Errorf("all three runs must be listed:\n%s", out)
	}
}

// TestHistoryPanelShowsHostOnlyWhenScopeIsWider pins that the HOST column
// earns its width. Scoped to one host every row would repeat the same name,
// spending cells the run list needs and telling the operator nothing they
// did not just choose.
func TestHistoryPanelShowsHostOnlyWhenScopeIsWider(t *testing.T) {
	one := histModelWith(t, 2)
	one.vp.width, one.vp.height = 120, 40
	if strings.Contains(one.histPanel(), "HOST") {
		t.Errorf("a single-host scope must not spend a HOST column:\n%s", one.histPanel())
	}

	two := one
	two.histScope = []string{"alpha", "beta"}
	if !strings.Contains(two.histPanel(), "HOST") {
		t.Errorf("a multi-host scope must name the host per row:\n%s", two.histPanel())
	}
}

// TestHistoryPanelMarksTheCursor pins that the run under the cursor is
// visibly the one enter would open.
func TestHistoryPanelMarksTheCursor(t *testing.T) {
	m := histModelWith(t, 3)
	m.vp.width, m.vp.height = 120, 40
	if !strings.Contains(m.histPanel(), ">") {
		t.Errorf("the cursor row must be marked:\n%s", m.histPanel())
	}
}

// TestHistoryPanelSaysWhenThereIsNothing pins the empty case: a host that has
// never been updated gets a sentence naming why the list is empty, never a
// bare frame that reads as a broken pane.
func TestHistoryPanelSaysWhenThereIsNothing(t *testing.T) {
	m, _ := send(testModel("alpha"), "H")
	mm, _ := m.Update(historyLoadedMsg{runs: nil})
	m2 := mm.(tuiModel)
	m2.vp.width, m2.vp.height = 120, 40

	out := m2.histPanel()
	if !strings.Contains(strings.ToLower(out), "no ") {
		t.Errorf("an empty history must explain itself:\n%s", out)
	}
}

// TestHistoryFrameNeverExceedsTheTerminal pins the height invariant for the
// NEW panel. The existing 1219-size sweep renders the host list, so it says
// nothing about this one — and bubbletea's renderer drops lines from the TOP
// of an over-tall frame, so a single row of overflow silently walks the
// banner off the screen. Both history states are swept: the run list, and the
// empty-history prose, which WRAPS and is therefore the more likely to grow.
//
// ANSI256 is set deliberately: init() pins termenv.Ascii, under which every
// style is a no-op and a frame carries no escape bytes at all — measuring a
// layout under it proves nothing.
func TestHistoryFrameNeverExceedsTheTerminal(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	full := histModelWith(t, 12)

	empty, _ := send(testModel("alpha"), "H")
	em, _ := empty.Update(historyLoadedMsg{runs: nil})
	emptyM := em.(tuiModel)

	for _, m := range []tuiModel{full, emptyM} {
		for _, h := range []int{6, 8, 10, 14, 20, 30, 50} {
			for _, w := range []int{40, 60, 80, 120, 200} {
				for _, logOpen := range []bool{false, true} {
					mm := m
					mm.vp.height, mm.vp.width = h, w
					mm.logOpen = logOpen
					got := strings.Count(mm.View(), "\n") + 1
					if got > h {
						t.Fatalf("frame is %d lines in a %dx%d terminal (logOpen=%v); "+
							"bubbletea drops from the TOP, so this eats the banner", got, w, h, logOpen)
					}
				}
			}
		}
	}
}

// TestEnterOpensTheRunIntoThePanes pins the drill-down: enter on a run fills
// the log pane with that capture and the stderr pane with its error
// projection, read through histindex so the TUI and `fleet history` can never
// disagree about what a capture contains.
func TestEnterOpensTheRunIntoThePanes(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha",
		"# fleet update — host=alpha started=x\n"+
			"03:47:14 === step dotfiles.sync (sync) ===\n"+
			"03:47:16 !! fatal: could not read Username\n"+
			"03:47:17 Already up to date.\n"+
			"# 2026-09-09T03:50:36Z finished\n")
	runs, err := historyRuns(dir, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}

	m, _ := send(testModel("alpha"), "H")
	mm, _ := m.Update(historyLoadedMsg{runs: runs})
	m2 := mm.(tuiModel)

	_, cmd := send(m2, "enter")
	if cmd == nil {
		t.Fatal("enter must return a Cmd — reading a capture is I/O and never happens in Update")
	}
	msg := cmd()
	opened, ok := msg.(historyOpenedMsg)
	if !ok {
		t.Fatalf("want historyOpenedMsg, got %T", msg)
	}

	m3m, _ := m2.Update(opened)
	m3 := m3m.(tuiModel)

	if !m3.histRunOpen() {
		t.Fatal("the run must be open after enter")
	}
	body := m3.logEntries()
	if len(body) != 3 {
		t.Fatalf("want the capture's 3 body lines, got %d: %+v", len(body), body)
	}
	var sawErr bool
	for _, e := range m3.errEntries() {
		if strings.Contains(e.line, "could not read Username") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("the stderr pane must show the capture's stderr projection")
	}
}

// TestEscUnwindsOneLevelAtATime pins the back-out: run -> list -> dashboard,
// and only THEN esc's existing meaning of clearing selection and search. An
// esc that dropped straight out of history would lose the operator's place
// in a list they may have scrolled a long way down.
func TestEscUnwindsOneLevelAtATime(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha", "beta"), "space", "H")
	mm, _ := m.Update(historyLoadedMsg{runs: runs})
	m2 := mm.(tuiModel)
	_, cmd := send(m2, "enter")
	om, _ := m2.Update(cmd().(historyOpenedMsg))
	open := om.(tuiModel)

	if !open.histRunOpen() {
		t.Fatal("precondition: a run is open")
	}
	back1, _ := send(open, "esc")
	if back1.histRunOpen() {
		t.Error("the first esc must close the run, not leave history")
	}
	if !back1.histOn {
		t.Error("the first esc must stay in history")
	}
	if len(back1.selected) == 0 {
		t.Error("the first esc must not clear the selection — it was unwinding history")
	}

	back2, _ := send(back1, "esc")
	if back2.histOn {
		t.Error("the second esc must leave history")
	}
	if len(back2.selected) == 0 {
		t.Error("the second esc leaves history; it must not also clear the selection")
	}

	back3, _ := send(back2, "esc")
	if len(back3.selected) != 0 {
		t.Error("once out of history, esc resumes its normal meaning and clears the selection")
	}
}

// TestOpeningARunNeverLosesLiveStreamLines pins the interaction the design
// flagged as the real risk: the log pane is shared with a running update.
// Viewing history must not discard lines the engine is still producing, so
// they keep buffering into m.logs and are shown again on the way out.
func TestOpeningARunNeverLosesLiveStreamLines(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	m := testModel("alpha")
	m.appendLog("alpha", "live line before history")

	m2, _ := send(m, "H")
	lm, _ := m2.Update(historyLoadedMsg{runs: runs})
	m3 := lm.(tuiModel)
	_, cmd := send(m3, "enter")
	om, _ := m3.Update(cmd().(historyOpenedMsg))
	open := om.(tuiModel)

	// a line arrives from the still-running update while history is on screen
	open.appendLog("alpha", "live line during history")

	for _, e := range open.logEntries() {
		if strings.Contains(e.line, "live line") {
			t.Fatalf("the live stream must not leak into the opened capture: %q", e.line)
		}
	}

	back, _ := send(open, "esc", "esc")
	var got []string
	for _, e := range back.logEntries() {
		got = append(got, e.line)
	}
	if len(got) != 2 {
		t.Fatalf("both live lines must survive the round trip, got %v", got)
	}
}
