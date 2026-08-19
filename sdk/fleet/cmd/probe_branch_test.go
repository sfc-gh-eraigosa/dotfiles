package cmd

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// countingRunner records every command AND how many times each host was dialled,
// so a test can prove the branch costs no extra SSH round-trip.
type countingRunner struct {
	mu    sync.Mutex
	out   map[string]string
	calls map[string]int
	cmds  []string
}

func (c *countingRunner) Run(host string, argv ...string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[host]++
	c.cmds = append(c.cmds, strings.Join(argv, " "))
	out, ok := c.out[host]
	if !ok {
		return "", runner.ErrFake
	}
	return out, nil
}
func (c *countingRunner) RunInteractive(string, ...string) error { return nil }
func (c *countingRunner) RunStdin(h, _ string, a ...string) (string, error) {
	return c.Run(h, a...)
}
func (c *countingRunner) RunVia(_, h string, a ...string) (string, error) { return c.Run(h, a...) }

// The streaming path is exercised by runner.Fake in the TUI tests; this fake
// only counts probe calls, so it returns an already-closed stream.
func (c *countingRunner) RunStream(string, string, ...string) (<-chan string, <-chan error) {
	lines := make(chan string)
	done := make(chan error, 1)
	close(lines)
	done <- nil
	return lines, done
}

// probeReply builds what a real host sends back: the stamp file, the delimiter,
// then the live branch.
func probeReply(stamp, liveBranch string) string {
	return stamp + "\n" + probeDelim + "\n" + liveBranch
}

func stampWithBranch(sha, branch string) string {
	return "commit=" + sha + "\ninstalled_at=1754700000\nbranch=" + branch + "\nhostname=h"
}

// F23a — the probe already spends one round-trip reading the stamp. Carrying
// the branch must ride along in it, not cost a second dial per host; on a
// 60-host fleet a second connection each would double the poll.
func TestBranchCostsNoExtraRoundTrip(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := &countingRunner{
		out:   map[string]string{"h1": probeReply(stampWithBranch(cur, "main"), "main")},
		calls: map[string]int{},
	}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	probeHost(sshconf.Host{Alias: "h1"}, r, base)

	if r.calls["h1"] != 1 {
		t.Fatalf("probe dialled the host %d times, want exactly 1", r.calls["h1"])
	}
	joined := strings.Join(r.cmds, " ")
	if !strings.Contains(joined, "rev-parse --abbrev-ref HEAD") {
		t.Fatalf("probe must ask for the live branch: %q", joined)
	}
	if !strings.Contains(joined, stampPath) {
		t.Fatalf("probe must still read the stamp: %q", joined)
	}
}

// F23b — splitting must not disturb the stamp half. The corrupt-stamp
// diagnosis is a real signal (a truncated write) and must survive verbatim.
func TestProbeSplitLeavesStampParsingIntact(t *testing.T) {
	cur := strings.Repeat("a", 40)
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	tests := []struct {
		name, reply, wantClass, wantNote, wantBranch string
	}{
		{
			name:      "good stamp and branch",
			reply:     probeReply(stampWithBranch(cur, "main"), "main"),
			wantClass: "up-to-date", wantBranch: "main",
		},
		{
			name:      "corrupt stamp still diagnosed",
			reply:     probeReply("commit=truncated", "main"),
			wantClass: "unknown", wantNote: "corrupt stamp", wantBranch: "main",
		},
		{
			name:      "mangled branch half must not affect the drift class",
			reply:     probeReply(stampWithBranch(cur, "main"), ""),
			wantClass: "up-to-date", wantBranch: "-",
		},
		{
			name:      "no delimiter at all (older host) still classifies",
			reply:     stampWithBranch(cur, "main"),
			wantClass: "up-to-date", wantBranch: "-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &countingRunner{out: map[string]string{"h": tc.reply}, calls: map[string]int{}}
			row := probeHost(sshconf.Host{Alias: "h"}, r, base)

			if row.Class != tc.wantClass {
				t.Errorf("class = %q, want %q", row.Class, tc.wantClass)
			}
			if tc.wantNote != "" && !strings.Contains(row.Note, tc.wantNote) {
				t.Errorf("note = %q, want it to contain %q", row.Note, tc.wantNote)
			}
			if got := branchCell(row); got != tc.wantBranch {
				t.Errorf("branch cell = %q, want %q", got, tc.wantBranch)
			}
		})
	}
}

// The stamp has carried `branch` since day one; probeHost discarded it.
func TestProbeKeepsTheInstalledBranchFromTheStamp(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := &countingRunner{
		out:   map[string]string{"h": probeReply(stampWithBranch(cur, "main"), "feature/x")},
		calls: map[string]int{},
	}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	row := probeHost(sshconf.Host{Alias: "h"}, r, base)
	if row.InstalledBranch != "main" {
		t.Errorf("InstalledBranch = %q, want main (from the stamp)", row.InstalledBranch)
	}
	if row.Branch != "feature/x" {
		t.Errorf("Branch = %q, want the LIVE checked-out branch", row.Branch)
	}
}

// F23e / F23d — the edges an operator will actually hit.
func TestBranchEdgeCases(t *testing.T) {
	cur := strings.Repeat("a", 40)
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	// These pin normalisation of the LIVE value. The stamped branch is held
	// equal to it so the mismatch marker (its own test) stays out of the way.
	tests := []struct{ name, live, want string }{
		{"detached HEAD is named, not echoed", "HEAD", "detached"},
		{"whitespace is trimmed", "  main \n", "main"},
		{"plain branch passes through", "feature/x", "feature/x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &countingRunner{
				out:   map[string]string{"h": probeReply(stampWithBranch(cur, tc.want), tc.live)},
				calls: map[string]int{},
			}
			row := probeHost(sshconf.Host{Alias: "h"}, r, base)
			if row.Branch != tc.want {
				t.Fatalf("Branch = %q, want %q", row.Branch, tc.want)
			}
			if got := branchCell(row); got != tc.want {
				t.Fatalf("branch cell = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("missing clone renders a dash", func(t *testing.T) {
		r := &countingRunner{
			out:   map[string]string{"h": probeReply(stampWithBranch(cur, "main"), "")},
			calls: map[string]int{},
		}
		row := probeHost(sshconf.Host{Alias: "h"}, r, base)
		if got := branchCell(row); got != "-" {
			t.Fatalf("branch cell = %q, want %q — an absent live branch must not\n"+
				"fall back to the stamped one, which would claim a clone exists", got, "-")
		}
	})
}

// The headless table is the scripted surface; the branch must be visible there
// too, not only in the TUI.
func TestRenderTableShowsTheBranchColumn(t *testing.T) {
	out := renderTable([]Row{
		{Alias: "h1", Class: "up-to-date", Commit: "0b8726e", Branch: "main", InstalledBranch: "main"},
		{Alias: "h2", Class: "ahead/divergent", Commit: "1bc1928", Branch: "feature/x", InstalledBranch: "main"},
	}, testNow)

	if !strings.Contains(out, "BRANCH") {
		t.Fatalf("table needs a BRANCH header:\n%s", out)
	}
	for _, want := range []string{"main", "feature/x"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}

func TestRenderJSONCarriesBothBranches(t *testing.T) {
	raw := renderJSON([]Row{{
		Alias: "h1", Class: "ahead/divergent",
		Branch: "feature/x", InstalledBranch: "main",
	}})
	for _, want := range []string{`"branch": "feature/x"`, `"installed_branch": "main"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("json missing %s:\n%s", want, raw)
		}
	}
}

// F23f — this is the whole targeting workflow: `/feature` then select-all then
// update. It only works if branch is part of what search matches against.
func TestSearchMatchesOnBranch(t *testing.T) {
	m := testModel("h1", "h2")
	m.setRow(Row{Alias: "h1", Class: "up-to-date", Branch: "main"})
	m.setRow(Row{Alias: "h2", Class: "ahead/divergent", Branch: "feature/x"})

	m.search.input = "feature"
	m.compileSearch()

	var matched []string
	for _, r := range m.rows {
		if m.matches(r) {
			matched = append(matched, r.Alias)
		}
	}
	if len(matched) != 1 || matched[0] != "h2" {
		t.Fatalf("matched %v, want only the feature-branch host", matched)
	}
}

// Adding the BRANCH column overflowed the row because failWidth budgeted from
// a hardcoded prefix that no longer matched the format string, and then a
// minimum-width floor pushed it past the edge anyway. Assert the invariant
// directly — no row may exceed the terminal — across the narrow widths where
// the arithmetic actually bites.
func TestRowNeverExceedsTerminalWidth(t *testing.T) {
	longCause := strings.Repeat("fatal: could not read from remote repository ", 6)

	// Scoped to the host ROWS, which is what this change touches, and to widths
	// a real terminal has. The static header and help lines are ~89 wide and
	// already overflowed below that before this work — a separate pre-existing
	// issue, deliberately not fixed here.
	for _, width := range []int{100, 120, 200} {
		m := testModel("host-with-a-long-alias")
		m.vp = viewport{height: 16, width: width}
		m.setRow(Row{
			Alias: "host-with-a-long-alias", Class: "unreachable",
			Branch: "feature/a-very-long-branch-name", InstalledBranch: "main",
		})
		m.updating = map[string]updState{"host-with-a-long-alias": {phase: updFail, log: longCause}}

		if w := lipgloss.Width(m.rowView(0)); w > width {
			t.Errorf("terminal width %d: row is %d wide: %q", width, w, m.rowView(0))
		}
	}
}

// F23c — the mismatch IS the signal: "checked out feature/x, installed main" is
// exactly what an operator is looking for, and it explains an otherwise
// mysterious ahead/divergent row. A matching pair must stay quiet.
func TestBranchCellMarksAMismatchAndStaysQuietOtherwise(t *testing.T) {
	same := branchCell(Row{Branch: "main", InstalledBranch: "main"})
	if same != "main" {
		t.Fatalf("matching branches should render plainly, got %q", same)
	}

	diff := branchCell(Row{Branch: "feature/x", InstalledBranch: "main"})
	if !strings.Contains(diff, "feature/x") {
		t.Fatalf("mismatch must show the LIVE branch, got %q", diff)
	}
	if diff == "feature/x" {
		t.Fatalf("mismatch must be marked so it is visibly different from a match, got %q", diff)
	}
	if !strings.Contains(diff, "main") {
		t.Fatalf("mismatch should name the installed branch too, got %q", diff)
	}
}
