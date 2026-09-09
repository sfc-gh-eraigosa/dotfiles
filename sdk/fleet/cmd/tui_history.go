package cmd

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/histindex"
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
