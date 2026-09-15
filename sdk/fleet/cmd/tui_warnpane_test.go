package cmd

import "testing"

// TestGitChatterAndGcloudNagStayOutOfTheErrorsPane pins Change A + Change B
// together: git's fetch/checkout stderr and gcloud's 3-line self-update nag
// both reach the log pane (every line, with its `!` gutter) but neither
// counts as a warning, so neither appears in the errors pane and neither
// raises the badge.
func TestGitChatterAndGcloudNagStayOutOfTheErrorsPane(t *testing.T) {
	m := testModel("a")
	chatter := []string{
		"From github.com:sfc-gh-eraigosa/dotfiles",
		" * branch            main       -> FETCH_HEAD",
		"Already on 'main'",
		"Updates are available for some Google Cloud CLI components.  To install them,",
		"please run:",
		"  $ gcloud components update",
	}
	for _, l := range chatter {
		m.appendLogLine("a", l, true)
	}

	if got := len(m.logs); got != len(chatter) {
		t.Fatalf("the log pane must keep every line, got %d want %d", got, len(chatter))
	}
	if got := m.errEntries(); len(got) != 0 {
		t.Fatalf("the errors pane must have none of this chatter, got %d: %+v", len(got), got)
	}
	if m.warns["a"] != 0 {
		t.Fatalf("none of this chatter is a warning, badge = %d", m.warns["a"])
	}
}

// TestARealWarningAppearsInBothPanes pins the other half: a genuine warning
// is neither chatter nor an advisory, so it shows up in the log pane (as
// always) AND in the errors pane, and raises the badge.
func TestARealWarningAppearsInBothPanes(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "installing", false)
	m.appendLogLine("a", "fatal: could not read Username for 'https://github.com'", true)

	found := false
	for _, e := range m.logs {
		if e.line == "fatal: could not read Username for 'https://github.com'" {
			found = true
		}
	}
	if !found {
		t.Fatal("the real warning must still be in the log pane")
	}
	errs := m.errEntries()
	if len(errs) != 1 || errs[0].line != "fatal: could not read Username for 'https://github.com'" {
		t.Fatalf("the real warning must be in the errors pane, got %+v", errs)
	}
	if m.warns["a"] != 1 {
		t.Fatalf("badge must count the real warning, got %d", m.warns["a"])
	}
}

// TestColourisedChatterIsClassifiedLikeTheCapture pins that the live path
// strips ANSI before classifying, as histindex.Read does for the same line on
// disk: a coloured benign/advisory line must not badge live yet count 0 in
// the history list.
func TestColourisedChatterIsClassifiedLikeTheCapture(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "\x1b[33mnpm warn deprecated glob@7.2.3: old\x1b[0m", true)
	m.appendLogLine("a", "\x1b[2mFrom github.com:o/r\x1b[0m", true)
	if m.warns["a"] != 0 || len(m.errEntries()) != 0 {
		t.Fatalf("coloured chatter counted: badge=%d pane=%d", m.warns["a"], len(m.errEntries()))
	}
}

// TestBadgeCountEqualsTheErrorsPaneLineCount pins the invariant that makes
// the badge trustworthy: whatever number the row shows is exactly the number
// of lines an operator finds by opening the errors pane, mixing chatter,
// advisories and real warnings from more than one host.
func TestBadgeCountEqualsTheErrorsPaneLineCount(t *testing.T) {
	m := testModel("h1", "h2")
	m.appendLogLine("h1", "Updates are available for some Google Cloud CLI components.  To install them,", true)
	m.appendLogLine("h1", "please run:", true)
	m.appendLogLine("h1", "  $ gcloud components update", true)
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h2", "From github.com:o/r", true)
	m.appendLogLine("h2", "npm warn install-scripts 2 packages have install scripts", true)
	m.appendLogLine("h2", "sudo: a password is required", true)

	lines, _ := m.liveWarnTotals()
	if got := len(m.errEntries()); got != lines {
		t.Fatalf("errEntries()=%d, liveWarnTotals lines=%d — badge and pane must agree", got, lines)
	}
	if lines != 2 {
		t.Fatalf("want exactly 2 real warnings (apt-get + sudo), got %d", lines)
	}
}
