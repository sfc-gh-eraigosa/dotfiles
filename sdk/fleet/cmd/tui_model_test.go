package cmd

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// Frames must be byte-stable without a TTY, so colour is pinned off for the
// whole package. Without this, goldens differ between CI and a dev terminal.
func init() { lipgloss.SetColorProfile(termenv.Ascii) }

func hosts(aliases ...string) []sshconf.Host {
	var out []sshconf.Host
	for _, a := range aliases {
		out = append(out, sshconf.Host{Alias: a})
	}
	return out
}

func testModel(aliases ...string) tuiModel {
	return newTUIModel(hosts(aliases...), runner.Fake{}, fakeBaseline{head: "abc"}, testNow, "main", 2, updplan.Default())
}

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// send routes keys through the real Update path.
func send(m tuiModel, keys ...string) (tuiModel, tea.Cmd) {
	var c tea.Cmd
	var mm tea.Model = m
	for _, k := range keys {
		mm, c = mm.Update(key(k))
	}
	return mm.(tuiModel), c
}

// --- F1: streaming -------------------------------------------------------

func TestInitFiresOneCmdPerHostAndBlocksNothing(t *testing.T) {
	m := testModel("a", "b", "c")
	if m.Init() == nil {
		t.Fatal("Init must return poll commands")
	}
	// Before any row arrives every host must read as pending, not as data.
	if len(m.pending) != 3 {
		t.Fatalf("expected 3 pending hosts, got %d", len(m.pending))
	}
	if !strings.Contains(m.View(), "polling") {
		t.Fatalf("unresolved hosts must render as polling:\n%s", m.View())
	}
}

func TestRowArrivalResortsButKeepsCursorOnItsAlias(t *testing.T) {
	m := testModel("aaa", "zzz")
	m.cursor = "zzz"
	// zzz resolves healthy; aaa unreachable. Worst-first puts aaa on top,
	// which would silently move an index-keyed cursor onto the wrong host.
	mm, _ := m.Update(hostRowMsg{row: Row{Alias: "zzz", Class: "up-to-date"}})
	mm, _ = mm.Update(hostRowMsg{row: Row{Alias: "aaa", Class: "unreachable"}})
	got := mm.(tuiModel)
	if got.rows[0].Alias != "aaa" {
		t.Fatalf("expected worst-first ordering, got %s first", got.rows[0].Alias)
	}
	if got.cursor != "zzz" {
		t.Fatalf("cursor must stay on its alias across a re-sort, got %q", got.cursor)
	}
	if len(got.pending) != 0 {
		t.Fatalf("resolved hosts must clear pending, got %v", got.pending)
	}
}

// --- F2: refresh ---------------------------------------------------------

func TestRefreshRepollsKeepingCursorAndSelection(t *testing.T) {
	m := testModel("a", "b")
	m.pending = map[string]bool{}
	m.selected["b"] = true
	m.cursor = "b"
	m2, cmd := send(m, "r")
	if cmd == nil {
		t.Fatal("r must issue poll commands")
	}
	if !m2.selected["b"] || m2.cursor != "b" {
		t.Fatal("refresh must preserve cursor and selection")
	}
	if len(m2.pending) != 2 {
		t.Fatalf("expected both hosts re-polled, got %v", m2.pending)
	}
}

// F2b — the in-flight ownership invariant: a host owned by the update engine
// must never be re-polled underneath it.
func TestRefreshSkipsHostsBeingUpdated(t *testing.T) {
	m := testModel("a", "b")
	m.pending = map[string]bool{}
	m.updating["a"] = updState{phase: updRunning}
	m2, _ := send(m, "r")
	if m2.pending["a"] {
		t.Fatal("a host being updated must not be re-polled (double ownership)")
	}
	if !m2.pending["b"] {
		t.Fatal("idle hosts must still refresh during an update")
	}
}

// --- F4: motion + paging -------------------------------------------------

func TestCursorClampsAtBothEnds(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "k", "k")
	if m2.indexOf(m2.cursor) != 0 {
		t.Fatal("cursor must clamp at the top")
	}
	m3, _ := send(m2, "j", "j", "j")
	if m3.indexOf(m3.cursor) != 1 {
		t.Fatal("cursor must clamp at the bottom")
	}
}

func TestEmptyFleetNeverPanicsOnAnyKey(t *testing.T) {
	m := testModel()
	for _, k := range []string{"j", "k", "g", "G", "u", "s", "v", " ", "n", "N", "/", "esc", "?", "?"} {
		m, _ = send(m, k)
	}
	if !strings.Contains(m.View(), "fleet discover") {
		t.Fatalf("empty fleet must guide the user:\n%s", m.View())
	}
}

func TestGGJumpsTopAndGJumpsBottom(t *testing.T) {
	m := testModel("a", "b", "c")
	m2, _ := send(m, "G")
	if m2.indexOf(m2.cursor) != 2 {
		t.Fatal("G must jump to the last row")
	}
	m3, _ := send(m2, "g", "g")
	if m3.indexOf(m3.cursor) != 0 {
		t.Fatal("gg must jump to the first row")
	}
}

// A lone `g` is not a motion — the next key cancels it. Without this, `g`
// followed by `j` would silently jump to the top instead of moving down.
func TestLoneGIsCancelledByTheNextKey(t *testing.T) {
	m := testModel("a", "b", "c")
	m2, _ := send(m, "G")  // bottom
	m3, _ := send(m2, "g") // pending
	m4, _ := send(m3, "k") // cancels the pending g and moves up
	if m4.indexOf(m4.cursor) != 1 {
		t.Fatalf("pending g must be cancelled by another key, cursor at %d", m4.indexOf(m4.cursor))
	}
}

func TestWindowResizeKeepsCursorVisible(t *testing.T) {
	m := testModel("a", "b", "c", "d", "e", "f", "g", "h")
	m2, _ := send(m, "G") // cursor at the last row
	mm, _ := m2.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	got := mm.(tuiModel)
	i := got.indexOf(got.cursor)
	if i < got.vp.top || i >= got.vp.top+got.visibleRows() {
		t.Fatalf("cursor %d outside viewport [%d,%d) after resize",
			i, got.vp.top, got.vp.top+got.visibleRows())
	}
}

func TestHalfPageMovesAndStaysInBounds(t *testing.T) {
	m := testModel("a", "b", "c", "d", "e", "f", "g", "h")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 13}) // 6 list rows once framed
	m2, _ := send(mm.(tuiModel), "l")                           // hide the log pane so the math is the list's alone
	m3, _ := send(m2, "ctrl+d")
	if m3.indexOf(m3.cursor) != 3 {
		t.Fatalf("ctrl+d should move half a page (3), got %d", m3.indexOf(m3.cursor))
	}
	m4, _ := send(m3, "ctrl+u")
	if m4.indexOf(m4.cursor) != 0 {
		t.Fatalf("ctrl+u should return to the top, got %d", m4.indexOf(m4.cursor))
	}
}

// --- F5/F6: search -------------------------------------------------------

func TestSearchIsSmartcaseAndHighlights(t *testing.T) {
	m := testModel("nano", "PI")
	m.rows = []Row{{Alias: "nano", Class: "behind"}, {Alias: "PI", Class: "up-to-date"}}
	m2, _ := send(m, "/", "n", "a")
	if !m2.matches(m2.rows[0]) {
		t.Fatal("all-lowercase pattern must match case-insensitively (smartcase)")
	}
	// An uppercase letter makes it case-sensitive.
	m3 := testModel()
	m3.rows = []Row{{Alias: "nano"}, {Alias: "NANO"}}
	m3, _ = send(m3, "/", "N", "A")
	if m3.matches(m3.rows[0]) {
		t.Fatal("a pattern with uppercase must be case-sensitive")
	}
}

// Keys typed in search mode are text — they must not act as motions.
func TestSearchModeSwallowsMotionKeys(t *testing.T) {
	m := testModel("a", "b", "c")
	start := m.cursor
	m2, _ := send(m, "/", "j", "j")
	if m2.search.input != "jj" {
		t.Fatalf("expected the runes to be typed, got %q", m2.search.input)
	}
	if m2.cursor != start {
		t.Fatal("motion keys must not move the cursor while searching")
	}
}

func TestInvalidRegexShowsErrorAndKeepsEditing(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "/", "[")
	if m2.search.err == "" {
		t.Fatal("an invalid pattern must surface an error")
	}
	if !strings.Contains(m2.View(), "bad pattern") {
		t.Fatalf("the error must be visible:\n%s", m2.View())
	}
	// Still editable — completing the class clears the error.
	m3, _ := send(m2, "a", "]")
	if m3.search.err != "" {
		t.Fatalf("completing the pattern must clear the error, got %q", m3.search.err)
	}
}

func TestEscClearsSearchEnterCommitsIt(t *testing.T) {
	m := testModel("alpha", "beta")
	m2, _ := send(m, "/", "b", "esc")
	if m2.search.input != "" || m2.mode != modeNormal {
		t.Fatal("esc must clear the search entirely")
	}
	m3, _ := send(m, "/", "b", "enter")
	if !m3.search.committed || m3.mode != modeNormal {
		t.Fatal("enter must commit the pattern and return to normal mode")
	}
}

func TestNextPrevMatchWrapsBothWays(t *testing.T) {
	m := testModel()
	m.rows = []Row{{Alias: "x1"}, {Alias: "no"}, {Alias: "x2"}}
	m.cursor = "x1"
	m, _ = send(m, "/", "x", "enter") // enter jumps to the next match: x2
	if m.cursor != "x2" {
		t.Fatalf("enter should jump to the next match, at %q", m.cursor)
	}
	m, _ = send(m, "n") // wraps to x1
	if m.cursor != "x1" {
		t.Fatalf("n must wrap to the first match, at %q", m.cursor)
	}
	m, _ = send(m, "N") // backwards wraps to x2
	if m.cursor != "x2" {
		t.Fatalf("N must wrap backwards, at %q", m.cursor)
	}
}

func TestNoMatchesReportsInsteadOfMoving(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "/", "z", "z", "enter")
	m3, _ := send(m2, "n")
	if m3.status != "no matches" {
		t.Fatalf("expected a no-matches status, got %q", m3.status)
	}
}

// --- F7: selection -------------------------------------------------------

func TestSpaceTogglesSelectionByAliasAcrossResort(t *testing.T) {
	m := testModel("aaa", "zzz")
	m.cursor = "zzz"
	m2, _ := send(m, " ")
	if !m2.selected["zzz"] {
		t.Fatal("space must select the cursor host")
	}
	// A re-sort that reorders rows must not move the selection to another host.
	mm, _ := m2.Update(hostRowMsg{row: Row{Alias: "aaa", Class: "unreachable"}})
	got := mm.(tuiModel)
	if !got.selected["zzz"] || got.selected["aaa"] {
		t.Fatalf("selection must be alias-keyed, got %v", got.selected)
	}
	m3, _ := send(got, " ")
	if m3.selected["zzz"] {
		t.Fatal("space must toggle off")
	}
}

func TestVisualRangeSelectsSpanAndCommits(t *testing.T) {
	m := testModel("a", "b", "c")
	m2, _ := send(m, "v", "j")
	if len(m2.selectedAliases()) != 2 {
		t.Fatalf("visual range should cover 2 rows, got %v", m2.selectedAliases())
	}
	m3, _ := send(m2, " ")
	if m3.vAnchor != nil || len(m3.selectedAliases()) != 2 {
		t.Fatalf("space must commit the range, got %v", m3.selectedAliases())
	}
}

func TestEscClearsSelection(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, " ", "j", " ")
	if len(m2.selectedAliases()) != 2 {
		t.Fatal("expected two selected")
	}
	m3, _ := send(m2, "esc")
	if len(m3.selectedAliases()) != 0 {
		t.Fatal("esc must clear the selection")
	}
}

// --- F8/F9/F13: the update engine ---------------------------------------

func TestUpdateTargetsSelectionElseCursor(t *testing.T) {
	m := testModel("a", "b", "c")
	if got := m.updateTargets(); len(got) != 1 || got[0] != m.cursor {
		t.Fatalf("with no selection the target is the cursor row, got %v", got)
	}
	m.selected["c"] = true
	m.selected["a"] = true
	got := m.updateTargets()
	if strings.Join(got, ",") != "a,c" {
		t.Fatalf("targets must be the selection in table order, got %v", got)
	}
}

func TestDecliningConfirmRunsNothing(t *testing.T) {
	m := testModel("a", "b")
	m.selected["a"] = true
	// u opens the answer form; three enters walk its fields to the confirm strip.
	mu, _ := send(m, "u")
	m2 := commitForm(mu)
	if m2.mode != modeConfirm {
		t.Fatal("u must reach a confirmation before running anything")
	}
	m3, cmd := send(m2, "n")
	if cmd != nil {
		t.Fatal("declining must not start any update")
	}
	if len(m3.updating) != 0 || m3.busy() {
		t.Fatalf("declining must leave the engine idle, got %v", m3.updating)
	}
	if !m3.selected["a"] {
		t.Fatal("declining must keep the selection")
	}
}

// F8d — the concurrency bound. More targets than slots must never exceed jobs.
func TestBackgroundWaveNeverExceedsJobLimit(t *testing.T) {
	m := testModel("a", "b", "c", "d", "e")
	m.jobs = 2
	for _, a := range []string{"a", "b", "c", "d", "e"} {
		m.selected[a] = true
	}
	mu, _ := send(m, "u")
	m2 := commitForm(mu)
	m3, _ := send(m2, "y")
	// Every host prechecks as background-capable.
	cur := m3
	for _, a := range []string{"a", "b", "c", "d", "e"} {
		mm, _ := cur.Update(precheckMsg{alias: a, interactive: false})
		cur = mm.(tuiModel)
		if cur.running > cur.jobs {
			t.Fatalf("running=%d exceeded jobs=%d", cur.running, cur.jobs)
		}
	}
	if cur.running != 2 {
		t.Fatalf("expected exactly jobs=2 running, got %d", cur.running)
	}
	if len(cur.bgQueue) != 3 {
		t.Fatalf("expected 3 hosts queued behind the slots, got %d", len(cur.bgQueue))
	}
	// A completion frees exactly one slot and admits exactly one queued host.
	mm, _ := cur.Update(bgUpdateDoneMsg{alias: "a"})
	after := mm.(tuiModel)
	if after.running != 2 || len(after.bgQueue) != 2 {
		t.Fatalf("a completion must refill one slot: running=%d queued=%d",
			after.running, len(after.bgQueue))
	}
}

// F8c — a failing host must not stall the wave, and must explain itself.
func TestFailedUpdateAdvancesTheWaveAndKeepsItsLog(t *testing.T) {
	m := testModel("a", "b")
	m.jobs = 1
	m.updating["a"] = updState{phase: updRunning}
	m.running = 1
	m.bgQueue = []string{"b"}
	mm, cmd := m.Update(bgUpdateDoneMsg{alias: "a", log: "Permission denied", err: errors.New("exit 1")})
	got := mm.(tuiModel)
	if got.updating["a"].phase != updFail {
		t.Fatal("a failing update must be marked FAIL")
	}
	if !strings.Contains(got.updating["a"].log, "Permission denied") {
		t.Fatalf("the failure must carry its cause, got %q", got.updating["a"].log)
	}
	if got.running != 1 || len(got.bgQueue) != 0 {
		t.Fatalf("the next host must take the freed slot: running=%d queued=%d",
			got.running, len(got.bgQueue))
	}
	if cmd == nil {
		t.Fatal("a completion must re-poll the host and pump the queue")
	}
	if !strings.Contains(got.View(), "FAIL") {
		t.Fatalf("the failure must be visible:\n%s", got.View())
	}
}

// F13a — sudo precheck routes each host to the lane it can actually run in.
func TestPrecheckRoutesInteractiveHostsToTheFallbackQueue(t *testing.T) {
	m := testModel("bg", "ia")
	m.jobs = 4
	mm, _ := m.Update(precheckMsg{alias: "bg", interactive: false})
	m2 := mm.(tuiModel)
	mm, _ = m2.Update(precheckMsg{alias: "ia", interactive: true})
	m3 := mm.(tuiModel)
	if m3.updating["bg"].phase != updRunning {
		t.Fatal("a passwordless host must start in the background immediately")
	}
	// The interactive host waits: its handoff would fight the running update
	// for the terminal.
	if len(m3.iaQueue) != 1 || m3.iaQueue[0] != "ia" {
		t.Fatalf("prompt-needing host must wait in the fallback queue, got %v", m3.iaQueue)
	}
}

func TestInteractiveFallbackRunsOnlyAfterTheWaveDrains(t *testing.T) {
	m := testModel("ia")
	m.jobs = 2
	m.iaQueue = []string{"ia"}
	m.running = 1 // a background update still holds the terminal
	if cmd := m.pump(); cmd != nil {
		t.Fatal("the fallback must not start while background updates run")
	}
	m.running = 0
	if cmd := m.pump(); cmd == nil {
		t.Fatal("the fallback must start once the wave drains")
	}
	if len(m.iaQueue) != 0 {
		t.Fatal("the fallback host must be dequeued when it starts")
	}
}

// F8f — the whole point: the UI keeps working while updates run.
func TestNavigationAndSearchWorkDuringUpdates(t *testing.T) {
	m := testModel("a", "b", "c")
	m.updating["a"] = updState{phase: updRunning}
	m.running = 1
	m2, _ := send(m, "j")
	if m2.indexOf(m2.cursor) != 1 {
		t.Fatal("navigation must work during a background update")
	}
	m3, _ := send(m2, "/", "b", "enter")
	if m3.mode != modeNormal || !m3.search.committed {
		t.Fatal("search must work during a background update")
	}
	if !m3.busy() {
		t.Fatal("the engine should still report busy")
	}
}

// --- F10: ssh action -----------------------------------------------------

func TestSSHIsBlockedWhileThatHostUpdates(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updRunning}
	if _, cmd := send(m, "s"); cmd != nil {
		t.Fatal("ssh must not race an in-flight update on the same host")
	}
	m2 := testModel("a")
	if _, cmd := send(m2, "s"); cmd == nil {
		t.Fatal("ssh must work on an idle host")
	}
}

// --- F11/quit guard ------------------------------------------------------

func TestHelpOverlayRendersEveryKeyAndClosesOnAnyKey(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "?")
	view := m2.View()
	for _, k := range keyHelp {
		if !strings.Contains(view, k.keys) {
			t.Fatalf("help overlay missing %q:\n%s", k.keys, view)
		}
	}
	m3, _ := send(m2, "j")
	if m3.mode != modeNormal {
		t.Fatal("any key must close the overlay")
	}
	if m3.indexOf(m3.cursor) != 0 {
		t.Fatal("the closing key must not also act as a motion")
	}
}

func TestQuitIsGuardedWhileUpdatesRun(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updRunning}
	m.running = 1
	m2, cmd := send(m, "q")
	if cmd != nil {
		t.Fatal("the first q must not quit while updates are in flight")
	}
	if !strings.Contains(m2.status, "q again") {
		t.Fatalf("the guard must tell the operator how to force it, got %q", m2.status)
	}
	if _, cmd := send(m2, "q"); cmd == nil {
		t.Fatal("a second q must force the quit")
	}
}
