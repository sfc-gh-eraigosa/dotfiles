package cmd

import (
	"time"

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
	err  error
}

// loadHistory reads the run list from inside a Cmd. Scanning a directory and
// summarising up to fifty captures per host is I/O, and I/O never happens in
// Update: a model built without a log dir (tests, the demo) would otherwise
// touch the filesystem on a keystroke, and a slow or unreadable mount would
// stall the whole UI rather than one message.
func loadHistory(dir string, hosts []string) tea.Cmd {
	return func() tea.Msg {
		runs, err := historyRuns(dir, hosts)
		return historyLoadedMsg{runs: runs, err: err}
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
}

// historyOpenedMsg carries one capture's parsed contents back into Update.
type historyOpenedMsg struct {
	path string
	// host travels with the message rather than being re-derived from the
	// filename: the run list already knows it, and decoding it twice is a
	// second place for the name to come out different.
	host string
	cap  histindex.Capture
	err  error
}

// openHistoryRun reads and parses one capture inside a Cmd. Reading a file is
// I/O, and I/O never happens in Update.
func openHistoryRun(r histindex.Summary) tea.Cmd {
	return func() tea.Msg {
		c, err := histindex.Read(r.Path)
		return historyOpenedMsg{path: r.Path, host: r.Host, cap: c, err: err}
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
func captureEntries(host string, c histindex.Capture, now time.Time) []logEntry {
	out := make([]logEntry, 0, len(c.Lines))
	for _, l := range c.Lines {
		out = append(out, logEntry{
			alias:  host,
			line:   l.Text,
			at:     now,
			stderr: l.Stderr,
			warn:   l.Stderr && !updexec.Benign(l.Text),
		})
	}
	return out
}
