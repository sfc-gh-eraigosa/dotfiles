package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/histindex"
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
	mm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
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
	mm, _ := m.Update(historyLoadedMsg{err: errTestScan, scope: m.histScope})
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
	mm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
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
	mm, _ := m.Update(historyLoadedMsg{runs: nil, scope: m.histScope})
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
	em, _ := empty.Update(historyLoadedMsg{runs: nil, scope: empty.histScope})
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
	mm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	m2 := mm.(tuiModel)

	m2, cmd := send(m2, "enter")
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
	mm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	m2 := mm.(tuiModel)
	m2, cmd := send(m2, "enter")
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
	lm, _ := m2.Update(historyLoadedMsg{runs: runs, scope: m2.histScope})
	m3 := lm.(tuiModel)
	m3, cmd := send(m3, "enter")
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

// TestOpenedRunFillsTheStderrPane pins a bug the demo frames caught that the
// unit tests missed: errEntries() correctly returned the capture's stderr,
// but the pane's EMPTINESS check reads errCount — a counter maintained
// incrementally as live lines arrive, and therefore zero for a capture that
// was read from disk. The pane rendered "stderr: none captured" directly
// underneath the very line it should have been showing.
func TestOpenedRunFillsTheStderrPane(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha",
		"# fleet update — host=alpha started=x\n"+
			"03:47:14 === step dotfiles.sync (sync) ===\n"+
			"03:47:16 !! fatal: could not read Username\n"+
			"# 2026-09-09T03:50:36Z finished\n")
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	m2 := lm.(tuiModel)
	m2.errOpen = true
	m2.vp.width, m2.vp.height = 100, 30

	open := openVia(t, m2, runs[0])

	if !open.errActive() {
		t.Error("a capture carrying stderr must make the error pane active")
	}
	out := open.View()
	if strings.Contains(out, "none captured") {
		t.Errorf("the stderr pane claims nothing was captured while showing a captured run:\n%s", out)
	}
	if !strings.Contains(stripANSI(out), "could not read Username") {
		t.Errorf("the capture's stderr line must appear in the pane:\n%s", out)
	}
}

// --- fixes for the PR #320 review -----------------------------------------

// TestTUIWiresTheCaptureDirectory pins the bug that made the whole feature a
// no-op in the shipped binary: m.logDir was declared and read in two places
// but NEVER assigned outside tests, so H scanned "" and always rendered "no
// captured runs". Worse, the same empty dir reaches beginStream, so with
// libs/log's "empty Dir means no capture" rule the dashboard's own updates
// were writing NO captures at all — a regression the CLI tests could not see
// because they inject a temp dir straight into historyRuns.
func TestTUIWiresTheCaptureDirectory(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	m := testModel("alpha")
	wireTUIPaths(&m)

	if m.logDir == "" {
		t.Fatal("logDir must be wired, or H scans nothing and TUI updates capture nothing")
	}
	if !strings.HasPrefix(m.logDir, state) {
		t.Errorf("logDir = %q, want it under the state dir %q", m.logDir, state)
	}
	if m.ansPath == "" {
		t.Error("wiring must still attach the answers path")
	}
}

// TestHistoryViewportFollowsTheRunCursor pins that the run list scrolls by its
// OWN offset. It was sliced by m.vp.top, which clampViewport recomputes from
// the HOST cursor: with the cursor deep in a long host list and a host with
// few captures, top exceeded the run count and the panel rendered a header
// with nothing under it.
func TestHistoryViewportFollowsTheRunCursor(t *testing.T) {
	m := histModelWith(t, 3)
	m.vp.width, m.vp.height = 100, 30
	m.vp.top = 24 // as if the host cursor were row 24 of a long fleet

	out := stripANSI(m.histPanel())
	if !strings.Contains(out, "finished") {
		t.Fatalf("the run list must render regardless of the host viewport:\n%s", out)
	}

	// and the cursor must stay reachable when scrolling down a long list
	long := histModelWith(t, 40)
	long.vp.width, long.vp.height = 100, 20
	for i := 0; i < 30; i++ {
		long, _ = send(long, "j")
	}
	if !strings.Contains(stripANSI(long.histPanel()), ">") {
		t.Error("the run cursor must remain visible when it moves past the pane height")
	}
}

// TestHTogglingOffClosesTheOpenedRun pins that H off is as complete as esc.
// Toggling off cleared histOn but not histPath, and histRunOpen() keys off
// histPath alone — so the dashboard came back with the stored capture still
// filling the log and stderr panes while a live update streamed unseen.
func TestHTogglingOffClosesTheOpenedRun(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	m2 := lm.(tuiModel)
	open := openVia(t, m2, runs[0])
	open.appendLog("alpha", "a live line")

	off, _ := send(open, "H")
	if off.histRunOpen() {
		t.Error("toggling history off must close the opened run")
	}
	if len(off.logEntries()) != 1 || !strings.Contains(off.logEntries()[0].line, "a live line") {
		t.Errorf("the panes must return to the live buffer, got %+v", off.logEntries())
	}
	if off.errTotal() != 0 {
		t.Errorf("errTotal must return to the live count, got %d", off.errTotal())
	}
}

// TestCaptureLinesKeepTheirRecordedTime pins that a stored line renders the
// time it was WRITTEN. Every line was stamped m.now (model construction), so
// a run captured over three minutes displayed as one instant repeated — and
// the stamp column exists precisely to show how long a step took.
func TestCaptureLinesKeepTheirRecordedTime(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha",
		"# fleet update — host=alpha started=x\n"+
			"03:47:14 first\n"+
			"03:50:21 later\n"+
			"# 2026-09-09T03:50:36Z finished\n")
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	open := openVia(t, lm.(tuiModel), runs[0])

	got := open.logEntries()
	if len(got) != 2 {
		t.Fatalf("want 2 lines, got %d", len(got))
	}
	if a, b := got[0].at.Format("15:04:05"), got[1].at.Format("15:04:05"); a == b {
		t.Errorf("both lines stamped %s — the recorded times were discarded", a)
	}
	// The file records UTC; the pane shows it in local time.
	want := time.Date(2026, 9, 9, 3, 47, 14, 0, time.UTC).In(time.Local).Format("15:04:05")
	if got[0].at.Format("15:04:05") != want {
		t.Errorf("first line stamped %s, want its recorded 03:47:14Z (%s local)", got[0].at.Format("15:04:05"), want)
	}
}

// TestCapitalJScrollsAnOpenedCapture pins the one J/K call site that was not
// converted to logEntries(). With no update running the live buffer is empty,
// so the clamp pinned logTop to 0 and J would not scroll a 300-line capture —
// while tab-focusing the pane and pressing j worked, making it look arbitrary.
func TestCapitalJScrollsAnOpenedCapture(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("# fleet update — host=alpha started=x\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "03:47:%02d line %d\n", i%60, i)
	}
	b.WriteString("# 2026-09-09T03:50:36Z finished\n")
	histCapture(t, dir, "20260909T030000Z", "alpha", b.String())
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	open := openVia(t, lm.(tuiModel), runs[0])
	open.logOpen = true
	open.vp.width, open.vp.height = 100, 30

	scrolled, _ := send(open, "J")
	if scrolled.logTop == 0 {
		t.Error("J must scroll an opened capture even when the live buffer is empty")
	}
}

// TestClosingARunRestoresLiveFollowing pins that leaving history hands the
// live pane back in a usable state. Opening a capture turns following OFF (a
// stored run is read from its start); esc restored neither, so the operator
// returned to a still-growing buffer frozen at line 1.
func TestClosingARunRestoresLiveFollowing(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	m2 := lm.(tuiModel)
	if !m2.logFollow {
		t.Skip("live following is off by default in this model; nothing to restore")
	}
	open := openVia(t, m2, runs[0])
	if open.logFollow {
		t.Fatal("precondition: opening a capture stops following")
	}

	back, _ := send(open, "esc")
	if !back.logFollow || back.logTop != 0 {
		t.Errorf("closing a run must resume following the live stream (follow=%v top=%d)",
			back.logFollow, back.logTop)
	}
}

// TestStderrTitleFollowsTheOpenedCapture pins the same class of mismatch this
// branch already fixed once for errCount: the pane BODY followed the capture
// while its TITLE still summed m.warns, a map only live lines ever write. An
// older clean capture displayed under a header counting a different run's
// warnings.
func TestStderrTitleFollowsTheOpenedCapture(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun) // clean: no stderr
	runs, _ := historyRuns(dir, []string{"alpha"})

	m := testModel("alpha")
	m.errOpen = true
	m.vp.width, m.vp.height = 100, 30
	// a live run that produced warnings
	m.appendLogLine("alpha", "WARNING: apt-get update failed", true)
	m.appendLogLine("alpha", "WARNING: grouped install failed", true)

	m2, _ := send(m, "H")
	lm, _ := m2.Update(historyLoadedMsg{runs: runs, scope: m2.histScope})
	open := openVia(t, lm.(tuiModel), runs[0])

	title := stripANSI(open.errViewN(6))
	if strings.Contains(title, "2 warning") {
		t.Errorf("the stderr title reports the LIVE run's warnings over a stored capture:\n%s", title)
	}
}

// TestWarnGutterAgreesWithTheRunListColumn pins that the `!` gutter and the
// list's WARN column classify identically. captureEntries passed Benign the
// RAW line while histindex.Read strips colour first, so a colourised benign
// git line counted as 0 in the list and was flagged as a warning once opened.
func TestWarnGutterAgreesWithTheRunListColumn(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha",
		"# fleet update — host=alpha started=x\n"+
			"03:47:16 !! \x1b[1;33mAlready on 'main'\x1b[0m\n"+
			"# 2026-09-09T03:50:36Z finished\n")
	runs, _ := historyRuns(dir, []string{"alpha"})
	if runs[0].Warnings != 0 {
		t.Fatalf("precondition: the colourised git line is benign, got %d", runs[0].Warnings)
	}

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	open := openVia(t, lm.(tuiModel), runs[0])

	for _, e := range open.logEntries() {
		if e.warn {
			t.Errorf("line flagged as a warning though the list counted none: %q", e.line)
		}
	}
}

// TestStaleHistoryLoadIsDropped pins that a slow scan cannot overwrite a newer
// one. Two H presses with different scopes race; if the first finishes second
// the list would show scope A's runs while histScope says B — which also flips
// the HOST-column decision and lets enter open a run outside the shown scope.
func TestStaleHistoryLoadIsDropped(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T040000Z", "beta", histRun)
	aRuns, _ := historyRuns(dir, []string{"alpha"})
	bRuns, _ := historyRuns(dir, []string{"beta"})

	m, _ := send(testModel("alpha", "beta"), "H") // scope: alpha (cursor)
	m2, _ := send(m, "H")                         // leave
	m3, _ := send(m2, "j", "H")                   // scope: beta
	scope := m3.histScope

	// beta's result lands first, then alpha's stale one arrives
	lm, _ := m3.Update(historyLoadedMsg{runs: bRuns, scope: scope})
	cur := lm.(tuiModel)
	sm, _ := cur.Update(historyLoadedMsg{runs: aRuns, scope: []string{"alpha"}})
	after := sm.(tuiModel)

	for _, r := range after.histRuns {
		if r.Host == "alpha" {
			t.Fatalf("a stale scan for a discarded scope overwrote the current list: %+v", after.histRuns)
		}
	}
}

// --- fixes for the second PR #320 review ----------------------------------

// openVia opens r the way the operator does — cursor on it, enter, and the
// read's answer — so the pending-open check is exercised, never bypassed.
func openVia(t *testing.T, m tuiModel, r histindex.Summary) tuiModel {
	t.Helper()
	m.histCursor = r.Path
	m, cmd := send(m, "enter")
	if cmd == nil {
		t.Fatal("precondition: enter must issue the open")
	}
	om, _ := m.Update(cmd())
	return om.(tuiModel)
}

// histOpen drives H → load → enter → open through the real Update path and
// returns the model with the run on screen.
func histOpen(t *testing.T, m tuiModel, runs []histindex.Summary) tuiModel {
	t.Helper()
	in, _ := send(m, "H")
	lm, _ := in.Update(historyLoadedMsg{runs: runs, scope: in.histScope})
	listed := lm.(tuiModel)
	listed, cmd := send(listed, "enter")
	if cmd == nil {
		t.Fatal("precondition: enter must issue the open")
	}
	om, _ := listed.Update(cmd().(historyOpenedMsg))
	return om.(tuiModel)
}

// TestLeavingHistoryWithoutOpeningKeepsTheLivePanes pins that a round trip
// through the run list costs the live panes nothing. closeHistoryRun ran on
// every exit and restored follow state that is only SAVED when a run opens —
// so H,H (or H,esc) during a streaming update restored the zero value, and
// both panes stopped following and jumped to line 1.
func TestLeavingHistoryWithoutOpeningKeepsTheLivePanes(t *testing.T) {
	for _, exit := range []string{"H", "esc"} {
		m := testModel("alpha")
		m.appendLog("alpha", "live")

		in, _ := send(m, "H")
		lm, _ := in.Update(historyLoadedMsg{runs: nil, scope: in.histScope})
		out, _ := send(lm.(tuiModel), exit)
		if !out.logFollow || !out.errFollow {
			t.Errorf("H then %s: live panes stopped following (log=%v err=%v)",
				exit, out.logFollow, out.errFollow)
		}

		// and a scrolled live pane keeps its place
		s := testModel("alpha")
		s.logFollow, s.logTop = false, 7
		s.errFollow, s.errTop = false, 3
		in2, _ := send(s, "H")
		out2, _ := send(in2, exit)
		if out2.logFollow || out2.logTop != 7 || out2.errFollow || out2.errTop != 3 {
			t.Errorf("H then %s: a scrolled live pane lost its place (log=%v/%d err=%v/%d)",
				exit, out2.logFollow, out2.logTop, out2.errFollow, out2.errTop)
		}
	}
}

// TestLiveEvictionLeavesAnOpenedCaptureWhereItIs pins that a busy update
// cannot scroll a capture the operator is reading. The eviction shift in
// appendLogLine corrects offsets into m.logs — but while a run is open,
// logTop/errTop index the CAPTURE, so every evicted live line walked the
// reader's view up by one.
func TestLiveEvictionLeavesAnOpenedCaptureWhereItIs(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("# fleet update — host=alpha started=x\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "03:47:%02d line %d\n", i%60, i)
		fmt.Fprintf(&b, "03:47:%02d !! warning %d\n", i%60, i)
	}
	b.WriteString("# 2026-09-09T03:50:36Z finished\n")
	histCapture(t, dir, "20260909T030000Z", "alpha", b.String())
	runs, _ := historyRuns(dir, []string{"alpha"})

	open := histOpen(t, testModel("alpha"), runs)
	open.logTop, open.errTop = 50, 20

	for i := 0; i < logCap+30; i++ {
		open.appendLogLine("alpha", fmt.Sprintf("live %d", i), i%2 == 0)
	}
	if open.logTop != 50 || open.errTop != 20 {
		t.Errorf("live eviction scrolled the opened capture: logTop=%d errTop=%d, want 50/20",
			open.logTop, open.errTop)
	}
}

// TestALateOpenAfterLeavingHistoryIsDropped pins that the async read cannot
// resurrect a run the operator already walked away from. With enter then a
// quick esc or H, the message landed after history was off and put the
// stored capture back into the dashboard's panes — the exact state the H-off
// fix exists to prevent.
func TestALateOpenAfterLeavingHistoryIsDropped(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	for _, exit := range []string{"H", "esc"} {
		m := testModel("alpha")
		m.appendLog("alpha", "live")
		in, _ := send(m, "H")
		lm, _ := in.Update(historyLoadedMsg{runs: runs, scope: in.histScope})
		listed := lm.(tuiModel)
		listed, cmd := send(listed, "enter")
		gone, _ := send(listed, exit)

		late, _ := gone.Update(cmd().(historyOpenedMsg))
		got := late.(tuiModel)
		if got.histRunOpen() {
			t.Errorf("enter then %s: the late open put a capture on the dashboard", exit)
		}
		if !got.logFollow {
			t.Errorf("enter then %s: the late open stopped the live pane following", exit)
		}
	}
}

// TestDoubleEnterStillRestoresLiveFollowing pins the other race: two enters
// before the read lands give two messages, and the second saved the ALREADY
// cleared follow state as the "live" one — so closing the run froze the live
// pane at line 1.
func TestDoubleEnterStillRestoresLiveFollowing(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha", histRun)
	runs, _ := historyRuns(dir, []string{"alpha"})

	in, _ := send(testModel("alpha"), "H")
	lm, _ := in.Update(historyLoadedMsg{runs: runs, scope: in.histScope})
	listed := lm.(tuiModel)
	listed, cmd1 := send(listed, "enter")
	listed, cmd2 := send(listed, "enter")

	var mm tea.Model = listed
	for _, c := range []tea.Cmd{cmd1, cmd2} {
		if c != nil {
			mm, _ = mm.Update(c())
		}
	}
	open := mm.(tuiModel)
	if !open.histRunOpen() {
		t.Fatal("precondition: the run is open")
	}
	back, _ := send(open, "esc")
	if !back.logFollow {
		t.Error("closing the run after a double enter must resume following the live stream")
	}
}

// TestSearchInHistoryLandsOnAMatchingRun pins that / n N search the list on
// screen. jumpMatch matched HOST rows and handed the host index to moveTo,
// which in history moves the RUN cursor — so /gam landed on whichever run
// happened to share gamma's host-row position.
func TestSearchInHistoryLandsOnAMatchingRun(t *testing.T) {
	dir := t.TempDir()
	histCapture(t, dir, "20260909T060000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T050000Z", "beta", histRun)
	histCapture(t, dir, "20260909T040000Z", "alpha", histRun)
	histCapture(t, dir, "20260909T030000Z", "beta", histRun)
	histCapture(t, dir, "20260909T010000Z", "gamma", histRun)

	m, _ := send(testModel("alpha", "beta", "gamma"), "a", "H")
	runs, _ := historyRuns(dir, m.histScope)
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	listed := lm.(tuiModel)

	found, _ := send(listed, "/", "g", "a", "m", "enter")
	r, ok := found.histAt()
	if !ok || r.Host != "gamma" {
		t.Errorf("/gam in history must land on gamma's run, got %+v", r)
	}
	again, _ := send(found, "n")
	if r2, _ := again.histAt(); r2.Host != "gamma" {
		t.Errorf("n must stay on the only gamma run, got %+v", r2)
	}
	if again.cursor != listed.cursor {
		t.Errorf("searching the run list moved the host cursor from %q to %q", listed.cursor, again.cursor)
	}
}

// TestOpeningARunKeepsItsRowOnScreen pins that the run cursor survives the
// list getting SHORTER. histTop was only adjusted by motion keys, and opening
// a run starts the log pane, which squeezes the list — so with the cursor on
// the last of nine runs the list showed runs 1-4 and no cursor at all.
func TestOpeningARunKeepsItsRowOnScreen(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 9; i++ {
		histCapture(t, dir, fmt.Sprintf("2026090%dT030000Z", i+1), "alpha", histRun)
	}
	runs, _ := historyRuns(dir, []string{"alpha"})

	m := testModel("alpha")
	m.logOpen = true
	m.vp.width, m.vp.height = 100, 30
	in, _ := send(m, "H")
	lm, _ := in.Update(historyLoadedMsg{runs: runs, scope: in.histScope})
	listed, _ := send(lm.(tuiModel), "G")
	listed, cmd := send(listed, "enter")
	om, _ := listed.Update(cmd().(historyOpenedMsg))
	open := om.(tuiModel)

	if open.visibleRows() >= len(runs) {
		t.Fatalf("precondition: the open run must squeeze the list below %d rows, got %d",
			len(runs), open.visibleRows())
	}
	if !strings.Contains(stripANSI(open.histPanel()), ">") {
		t.Errorf("the opened run's row scrolled out of the list:\n%s", stripANSI(open.histPanel()))
	}
	// and moving afterwards does not jump the window
	up, _ := send(open, "k")
	if !strings.Contains(stripANSI(up.histPanel()), ">") {
		t.Errorf("k after opening lost the cursor:\n%s", stripANSI(up.histPanel()))
	}
}

// TestCaptureStampsRenderInLocalTime pins that an opened run speaks the same
// clock as everything around it. libs/log writes line stamps in UTC; the run
// list's WHEN column and live lines are local, so in PDT a run listed at
// 20:00 opened with lines stamped 03:47.
func TestCaptureStampsRenderInLocalTime(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("PDT", -7*3600)
	t.Cleanup(func() { time.Local = saved })

	dir := t.TempDir()
	histCapture(t, dir, "20260909T030000Z", "alpha",
		"# fleet update — host=alpha started=x\n"+
			"03:47:14 first\n"+
			"# 2026-09-09T03:50:36Z finished\n")
	runs, _ := historyRuns(dir, []string{"alpha"})
	open := histOpen(t, testModel("alpha"), runs)

	if got := open.logEntries()[0].at.Format("15:04:05"); got != "20:47:14" {
		t.Errorf("line stamped %s, want 20:47:14 (03:47:14Z in PDT)", got)
	}
}

// TestUnreadableCaptureIsNotCalledUnfinished pins the "never invent a fact"
// rule for a row whose file could not be read. It got a zero summary and
// rendered as "unfinished -" — a result and a warning count the file never
// supported.
func TestUnreadableCaptureIsNotCalledUnfinished(t *testing.T) {
	dir := t.TempDir()
	// a dangling symlink: listed by Scan, unreadable by Read, even as root
	if err := os.Symlink(filepath.Join(dir, "gone"),
		filepath.Join(dir, "20260909T030000Z__alpha.log")); err != nil {
		t.Fatal(err)
	}
	runs, err := historyRuns(dir, []string{"alpha"})
	if err != nil || len(runs) != 1 {
		t.Fatalf("precondition: one unreadable run listed, got %v %+v", err, runs)
	}

	m, _ := send(testModel("alpha"), "H")
	lm, _ := m.Update(historyLoadedMsg{runs: runs, scope: m.histScope})
	listed := lm.(tuiModel)
	listed.vp.width, listed.vp.height = 120, 40

	out := stripANSI(listed.histPanel())
	if strings.Contains(out, "unfinished") {
		t.Errorf("an unreadable capture must not claim the run was unfinished:\n%s", out)
	}
	if !strings.Contains(out, "unreadable") {
		t.Errorf("an unreadable capture must say so:\n%s", out)
	}
}
