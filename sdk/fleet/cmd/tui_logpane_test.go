package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// --- (1) prompt forward when the credential is empty ----------------------

// A remembered non-secret answer must NOT let a wave start with no credential:
// every privileged step would silently skip and the operator would never have
// been asked for the one answer that cannot be defaulted.
func TestEmptyCredentialAlwaysPromptsEvenWhenOtherAnswersRemembered(t *testing.T) {
	m := testModel("a")
	m.ans = answers{windows: "s", gemini: "keep"} // remembered, but no credential
	m2, _ := send(m, "u")
	if m2.mode != modeAnswers {
		t.Fatalf("an empty credential must prompt forward, mode=%v", m2.mode)
	}
	if m2.ansField != fieldSudo {
		t.Fatalf("it must land on the credential field, got %v", m2.ansField)
	}
	// The remembered answers must survive being prompted.
	if m2.ans.windows != "s" || m2.ans.gemini != "keep" {
		t.Fatalf("prompting must not discard remembered answers: %+v", m2.ans)
	}
}

// With one set, `u` goes straight to confirm — `e` is the way back in.
func TestPopulatedCredentialSkipsToConfirmAndEEdits(t *testing.T) {
	m := testModel("a")
	m.ans = answers{sudoSecret: "xx", windows: "s"}
	m2, _ := send(m, "u")
	if m2.mode != modeConfirm {
		t.Fatalf("a populated credential should skip to confirm, mode=%v", m2.mode)
	}
	m3, _ := send(m2, "e")
	if m3.mode != modeAnswers || m3.ansField != fieldSudo {
		t.Fatal("`e` must reopen the form on the credential field")
	}
	if m3.ans.secretLen() != 2 {
		t.Fatal("editing must not discard the existing credential")
	}
}

// --- (2) the streaming log pane -------------------------------------------

func TestLogPaneTogglesAndRestoresFullHeight(t *testing.T) {
	m := testModel("a", "b", "c")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	full := mm.(tuiModel)
	// Baseline = the list with the pane hidden entirely.
	hidden, _ := send(full, "l")
	fullRows := hidden.visibleRows()

	// The pane ships ON, so it is discoverable without reading the help.
	if !full.logOpen {
		t.Fatal("the log pane must be on by default")
	}
	// ...but while it has nothing to show it collapses to one hint line rather
	// than claiming a fifth of the screen to display an empty box.
	if full.logActive() {
		t.Fatal("an empty pane must not be active")
	}
	emptyRows := full.visibleRows()

	// Once output arrives it takes its share and the list shrinks.
	full.appendLog("a", "Installing...")
	if !full.logActive() || full.logHeight() < 3 {
		t.Fatalf("with output the pane must claim height, got %d", full.logHeight())
	}
	if full.visibleRows() >= emptyRows {
		t.Fatalf("the list must shrink once logs flow (%d -> %d)", emptyRows, full.visibleRows())
	}

	closed, _ := send(full, "l")
	if closed.logOpen {
		t.Fatal("`l` must hide the pane")
	}
	if closed.visibleRows() != fullRows {
		t.Fatalf("hiding must restore the full-height list (%d != %d)",
			closed.visibleRows(), fullRows)
	}
	reopened, _ := send(closed, "l")
	if !reopened.logOpen {
		t.Fatal("`l` again must show it")
	}
}

// Even on a short terminal the split must leave both halves usable rather
// than collapsing the list to zero rows.
func TestSplitKeepsBothHalvesUsableOnASmallTerminal(t *testing.T) {
	m := testModel("a", "b")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	open := mm.(tuiModel)
	open.appendLog("a", "line") // make the pane active
	if open.visibleRows() < 1 || open.logHeight() < 1 {
		t.Fatalf("split collapsed: list=%d log=%d", open.visibleRows(), open.logHeight())
	}
}

// Lines arrive as messages and are tagged with the host, so a concurrent wave
// stays readable when several installs interleave.
func TestStreamedLinesAreTaggedAndRendered(t *testing.T) {
	m := testModel("a", "b")
	m.logOpen = true
	mm, _ := m.Update(logLineMsg{alias: "a", line: "Installing sops..."})
	mm, _ = mm.(tuiModel).Update(logLineMsg{alias: "b", line: "Updating goenv..."})
	got := mm.(tuiModel)
	if len(got.logs) != 2 {
		t.Fatalf("expected 2 buffered lines, got %d", len(got.logs))
	}
	view := got.View()
	for _, want := range []string{"Installing sops", "Updating goenv", "a", "b"} {
		if !strings.Contains(view, want) {
			t.Fatalf("log pane missing %q:\n%s", want, view)
		}
	}
}

// A line must re-issue the reader, or the stream stops after one line.
func TestEachLineReissuesTheReader(t *testing.T) {
	m := testModel("a")
	m.streams["a"] = stream{lines: make(chan string), done: make(chan error, 1)}
	_, cmd := m.Update(logLineMsg{alias: "a", line: "x"})
	if cmd == nil {
		t.Fatal("receiving a line must re-issue the reader or the stream stalls")
	}
}

// The buffer is capped: a fleet-wide install emits far more than fits.
func TestLogBufferIsCapped(t *testing.T) {
	m := testModel("a")
	for i := 0; i < logCap+50; i++ {
		m.appendLog("a", "line")
	}
	if len(m.logs) > logCap {
		t.Fatalf("log buffer must stay capped, got %d", len(m.logs))
	}
}

// Scrolling must stop the tail yanking the view away mid-read; G re-follows.
func TestScrollingPausesFollowAndGResumes(t *testing.T) {
	m := testModel("a")
	m.logOpen = true
	for i := 0; i < 50; i++ {
		m.appendLog("a", "l")
	}
	scrolled, _ := send(m, "J")
	if scrolled.logFollow {
		t.Fatal("scrolling must pause following")
	}
	// G is the host-list "last row" key; while the pane follows it also means
	// "back to the tail", which is the vim-ish reading of G in a pager.
	back, _ := send(scrolled, "l")
	back, _ = send(back, "l")
	if !back.logFollow {
		t.Fatal("re-opening the pane should resume following")
	}
}

// The row's FAIL text still comes from the stream, so the existing per-host
// error reporting keeps working now that output is not captured at the end.
func TestFailureTextStillComesFromTheStreamedTail(t *testing.T) {
	m := testModel("a")
	m.updating["a"] = updState{phase: updRunning}
	m.running = 1
	m.appendLog("a", "error: pathspec 'main' did not match")
	m.appendLog("a", "hint: Disable this message with \"git config advice.x false\"")
	mm, _ := m.Update(bgUpdateDoneMsg{alias: "a", err: runner.ErrFake})
	got := mm.(tuiModel)
	st := got.updating["a"]
	if st.phase != updFail {
		t.Fatalf("expected FAIL, got %v", st.phase)
	}
	if !strings.Contains(st.log, "pathspec") {
		t.Fatalf("the row must explain itself from the stream: %q", st.log)
	}
	if strings.Contains(strings.ToLower(st.log), "hint:") {
		t.Fatalf("git advice must still be filtered: %q", st.log)
	}
}

// --- status dot + panel framing ------------------------------------------

// The dot means one thing at a time: navy = chosen, green = succeeded,
// red = failed. Outcome outranks selection, so a finished wave can be read
// without looking at the UPDATE column.
func TestSelectionDotReflectsOutcome(t *testing.T) {
	m := testModel("a", "b", "c")
	m.selected["a"] = true
	m.updating["b"] = updState{phase: updOK}
	m.updating["c"] = updState{phase: updFail, log: "boom"}
	m.selected["c"] = true // selected AND failed -> the failure must win

	idx := map[string]int{}
	for i, r := range m.rows {
		idx[r.Alias] = i
	}
	if got := m.markFor(idx["a"]); !strings.Contains(got, "●") {
		t.Fatalf("a selected host needs a dot, got %q", got)
	}
	// Distinct styling per outcome (ASCII profile strips colour, so compare
	// the three against each other rather than asserting escape codes).
	sel, ok, fail := m.markFor(idx["a"]), m.markFor(idx["b"]), m.markFor(idx["c"])
	for _, s := range []string{sel, ok, fail} {
		if !strings.Contains(s, "●") {
			t.Fatalf("every state should render a dot, got %q", s)
		}
	}
	// An unselected, un-updated host has no dot at all.
	m2 := testModel("z")
	if got := m2.markFor(0); strings.Contains(got, "●") {
		t.Fatalf("an idle host must not show a dot, got %q", got)
	}
}

// The dot must not butt against the hostname.
func TestSpaceBetweenDotAndHostname(t *testing.T) {
	m := testModel("alpha")
	m.selected["alpha"] = true
	row := stripANSI(m.rowView(0))
	i := strings.Index(row, "●")
	if i < 0 {
		t.Fatalf("expected a dot in %q", row)
	}
	if row[i+len("●")] != ' ' {
		t.Fatalf("the dot needs a space before the hostname: %q", row)
	}
}

// Each area is its own framed panel, so the screen reads as separated
// sections rather than one run-on block.
func TestSectionsAreFramedAsPanels(t *testing.T) {
	m := testModel("a", "b")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	base := mm.(tuiModel)

	// host list + collapsed log line
	view := base.View()
	if strings.Count(view, "╭") < 2 {
		t.Fatalf("expected the list and the log line to be framed:\n%s", view)
	}
	// help overlay
	helped, _ := send(base, "?")
	if !strings.Contains(helped.View(), "╭") {
		t.Fatalf("the help overlay must be framed:\n%s", helped.View())
	}
	// active log pane
	base.appendLog("a", "working")
	if strings.Count(base.View(), "╭") < 2 {
		t.Fatalf("the active log pane must be framed:\n%s", base.View())
	}
}

// The banner is the first thing on screen: it must say what the tool is, what
// build is running, and what the keys do — a bare key strip said none of that.
func TestBannerShowsIdentityVersionAndKeys(t *testing.T) {
	m := testModel("a")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	head := mm.(tuiModel).banner()

	if !strings.Contains(head, "fleet") {
		t.Fatalf("the banner must name the tool:\n%s", head)
	}
	if !strings.Contains(head, Version) {
		t.Fatalf("the banner must carry the version %q:\n%s", Version, head)
	}
	// Every header-flagged key has to appear across the banner's two hint rows,
	// or a key is once again implemented but invisible.
	for _, k := range keyHelp {
		if k.hdr && !strings.Contains(head, k.keys+":") {
			t.Fatalf("banner omits header key %q:\n%s", k.keys, head)
		}
	}
	if !strings.Contains(head, "╭") {
		t.Fatalf("the banner must be framed:\n%s", head)
	}
}

// Narrow terminals must not wrap the banner into a mess.
func TestBannerFitsNarrowTerminals(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120} {
		m := testModel("a")
		mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		head := stripANSI(mm.(tuiModel).banner())
		for _, line := range strings.Split(head, "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: banner line is %d wide: %q", w, got, line)
			}
		}
	}
}
