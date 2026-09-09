package cmd

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/histindex"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
)

// historyRuns loads the captured runs for hosts from dir, newest first, with
// each run's summary already read.
//
// Scope follows the SELECTION, exactly as `u` and `w` do through
// updateTargets(): an operator who selected three hosts means those three,
// and one who selected none means the host under the cursor. An empty list
// means the whole fleet — the fresh-session case, where there is nothing to
// scope by yet.
//
// It returns Summary rather than Run because the list has to be triageable
// without opening anything: whether the run finished and how many warnings
// it carries are the two facts a filename cannot supply, and both need the
// file read. Retention caps captures at 50 per host, so the read cost is
// bounded by the scope the operator chose.
//
// A missing directory is an EMPTY history, never an error — the dashboard on
// a machine that has never run an update must open history to a sentence
// rather than a failure, the same rule fleet applies to a missing
// ~/.ssh/config.
func historyRuns(dir string, hosts []string) ([]histindex.Summary, error) {
	runs, err := histindex.Scan(dir)
	if err != nil {
		return nil, err
	}

	want := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		want[h] = true
	}

	out := make([]histindex.Summary, 0, len(runs))
	for _, r := range runs {
		if len(want) > 0 && !want[r.Host] {
			continue
		}
		s, err := histindex.Summarize(r)
		if err != nil {
			// One unreadable capture must not cost the whole list: the row
			// still names the run, it just carries no summary.
			s = histindex.Summary{Run: r}
		}
		out = append(out, s)
	}
	return out, nil
}

// historyLoadedMsg carries the scan result back into Update.
type historyLoadedMsg struct {
	runs []histindex.Summary
	// scope identifies WHICH request this answers. Two H presses with
	// different scopes can be in flight at once, and without this the slower
	// one overwrites the newer list — leaving histScope describing one set of
	// hosts while the rows show another, which also flips the HOST-column
	// decision and lets enter open a run outside the displayed scope.
	scope []string
	err   error
}

// sameScope reports whether two scope snapshots are the same request.
func sameScope(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// loadHistory reads the run list from inside a Cmd. Scanning a directory and
// summarising up to fifty captures per host is I/O, and I/O never happens in
// Update: a model built without a log dir (tests, the demo) would otherwise
// touch the filesystem on a keystroke, and a slow or unreadable mount would
// stall the whole UI rather than one message.
func loadHistory(dir string, hosts []string) tea.Cmd {
	return func() tea.Msg {
		runs, err := historyRuns(dir, hosts)
		return historyLoadedMsg{runs: runs, scope: hosts, err: err}
	}
}

// listLen is how many rows the ACTIVE list has — host rows normally, runs in
// history. Motion keys compute their targets from it, which is what lets
// j/k/gg/G/ctrl+d serve both lists without a second key table.
func (m tuiModel) listLen() int {
	if m.histOn {
		return len(m.histRuns)
	}
	return len(m.rows)
}

// histIndexOf locates a run by PATH, the run list's stable key.
func (m tuiModel) histIndexOf(path string) int {
	for i, r := range m.histRuns {
		if r.Path == path {
			return i
		}
	}
	return -1
}

// histMoveTo clamps like moveTo: motion may not run off either end.
func (m *tuiModel) histMoveTo(i int) {
	if len(m.histRuns) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i > len(m.histRuns)-1 {
		i = len(m.histRuns) - 1
	}
	m.histCursor = m.histRuns[i].Path
	m.clampHistViewport(i)
}

// clampHistViewport keeps the run cursor on screen. The run list has its own
// offset because m.vp belongs to the HOST list and clampViewport recomputes
// it from the host cursor — slicing the runs by it rendered a header with
// nothing under it, and pinned the cursor off-screen with no way to reach it.
func (m *tuiModel) clampHistViewport(i int) {
	h := m.visibleRows()
	if h < 1 {
		h = 1
	}
	if i < m.histTop {
		m.histTop = i
	}
	if i >= m.histTop+h {
		m.histTop = i - h + 1
	}
	if m.histTop < 0 {
		m.histTop = 0
	}
}

// historyOpenedMsg carries one capture's parsed contents back into Update.
type historyOpenedMsg struct {
	path string
	// host travels with the message rather than being re-derived from the
	// filename: the run list already knows it, and decoding it twice is a
	// second place for the name to come out different.
	host string
	// day is the run's date, so a captured line can be stamped with the time
	// it was actually written rather than with the moment the TUI started.
	day time.Time
	cap histindex.Capture
	err error
}

// openHistoryRun reads and parses one capture inside a Cmd. Reading a file is
// I/O, and I/O never happens in Update.
func openHistoryRun(r histindex.Summary) tea.Cmd {
	return func() tea.Msg {
		c, err := histindex.Read(r.Path)
		return historyOpenedMsg{path: r.Path, host: r.Host, day: r.At, cap: c, err: err}
	}
}

// histAt returns the run under the run cursor.
func (m tuiModel) histAt() (histindex.Summary, bool) {
	i := m.histIndexOf(m.histCursor)
	if i < 0 {
		return histindex.Summary{}, false
	}
	return m.histRuns[i], true
}

// histRunOpen reports whether a capture is on screen rather than the list.
// It is the second level of history, and esc unwinds it before leaving.
func (m tuiModel) histRunOpen() bool { return m.histPath != "" }

// logEntries is the ACTIVE source for the stream panes: the opened capture
// when a run is on screen, the live buffer otherwise.
//
// The live buffer is never touched by history. An update still running keeps
// appending to m.logs the whole time a capture is being read, and closing the
// run shows it again with nothing missing — the pane is shared, so viewing
// the past must not cost the operator the present.
func (m tuiModel) logEntries() []logEntry {
	if m.histRunOpen() {
		return m.histLines
	}
	return m.logs
}

// captureEntries converts a parsed capture into the same logEntry the stream
// panes already render, so the panes need no notion of where their lines came
// from and the two sources cannot drift into two renderers.
func captureEntries(host string, c histindex.Capture, day time.Time) []logEntry {
	out := make([]logEntry, 0, len(c.Lines))
	for _, l := range c.Lines {
		out = append(out, logEntry{
			alias:  host,
			line:   l.Text,
			at:     stampOf(day, l.Time),
			stderr: l.Stderr,
			// Colour is stripped BEFORE classifying, exactly as
			// histindex.Read does for the WARN column. Passing the raw line
			// made a colourised benign git line count as 0 in the run list
			// and yet raise the `!` gutter once the run was opened — the two
			// views disagreeing about the same line.
			warn: l.Stderr && !updexec.Benign(ansi.Strip(l.Text)),
		})
	}
	return out
}

// stampOf rebuilds a captured line's wall-clock time from the run's date and
// the line's "HH:MM:SS" prefix. The panes render this column to show how long
// a step took; stamping every line with the model's construction time made a
// three-minute run read as a single instant repeated.
//
// A line with no parsable stamp keeps the run's own time rather than being
// dropped or zeroed — output written before the first stamp is still output.
func stampOf(day time.Time, clock string) time.Time {
	if clock == "" {
		return day
	}
	t, err := time.Parse("15:04:05", clock)
	if err != nil {
		return day
	}
	return time.Date(day.Year(), day.Month(), day.Day(),
		t.Hour(), t.Minute(), t.Second(), 0, day.Location())
}

// wireTUIPaths attaches the on-disk locations the model needs. It exists as
// one function because logDir was declared, read in two places, and never
// assigned: H scanned "" and always rendered "no captured runs", and the same
// empty string reached beginStream, where libs/log's "an empty Dir means no
// capture" rule meant the dashboard's own updates wrote NOTHING. Both paths
// looked fine in tests, which inject a temp directory directly.
//
// Wiring stays HERE rather than in newTUIModel so the model remains a pure
// value and tests never touch a real config or state directory.
func wireTUIPaths(m *tuiModel) {
	m.ansPath = answersPath()
	m.ans = loadAnswers(m.ansPath)
	m.logDir = fleetLogDir()
}

// closeHistoryRun returns the stream panes to the live buffer, restoring the
// follow state the operator had before the capture was opened. Both exits
// from an open run (esc, and toggling H off) go through it, so neither can
// leave the panes showing a stored capture over a running update.
func (m *tuiModel) closeHistoryRun() {
	m.histPath, m.histLines, m.histErrCount = "", nil, 0
	m.logFollow, m.errFollow = m.liveFollow, m.liveErrFollow
	m.logTop, m.errTop = 0, 0
}
