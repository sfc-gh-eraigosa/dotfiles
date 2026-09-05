package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// --- force reset to origin ------------------------------------------------

// The reset is destructive, so it must preserve first. `git reset --hard`
// discards local commits AND uncommitted files; the host's entire current
// state is committed to a rescue branch before the reset, the same guarantee
// `--force` makes for a dirty clone.
func TestForceResetPreservesTheHostsStateFirst(t *testing.T) {
	repo := updplan.Default().Repos["dotfiles"]
	s, err := updexec.SyncScript(repo, updplan.LocalSkip, true)
	if err != nil {
		t.Fatal(err)
	}
	reset := strings.Index(s, "git reset --hard")
	rescue := strings.Index(s, "fleet-reset/")
	if reset < 0 {
		t.Fatalf("reset requested but not performed:\n%s", s)
	}
	if rescue < 0 || rescue > reset {
		t.Fatalf("the host's state must be saved BEFORE the reset:\n%s", s)
	}
	if !strings.Contains(s, "git add -A") {
		t.Fatalf("untracked files must be preserved too (a stash tree omits them):\n%s", s)
	}
	// It resets to what was fetched, never to a second network call.
	if !strings.Contains(s, "git reset --hard FETCH_HEAD") {
		t.Fatalf("reset must use the already-fetched commit:\n%s", s)
	}
}

// Unset or "n" leaves the safe fast-forward path exactly as it was.
func TestWithoutResetTheUpdateStaysFastForwardOnly(t *testing.T) {
	repo := updplan.Default().Repos["dotfiles"]
	s, err := updexec.SyncScript(repo, updplan.LocalSkip, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "reset --hard") {
		t.Fatalf("an un-requested reset must never appear:\n%s", s)
	}
	if !strings.Contains(s, "merge --ff-only FETCH_HEAD") {
		t.Fatalf("default must stay fast-forward-only:\n%s", s)
	}
	if (answers{}).forceReset() || (answers{reset: "n"}).forceReset() {
		t.Fatal("only an explicit y may force a reset")
	}
}

// The answer has to reach the host, and the confirm gate has to warn — this is
// the one answer that can destroy work.
func TestResetAnswerReachesTheHostAndIsWarnedAbout(t *testing.T) {
	m := testModel("a")
	m.ans = answers{sudoSecret: "xx", reset: "y"}
	m2, _ := send(m, "u") // credential set -> straight to confirm
	if m2.mode != modeConfirm {
		t.Fatalf("expected confirm, mode=%v", m2.mode)
	}
	view := stripANSI(m2.View())
	if !strings.Contains(view, "force reset") {
		t.Fatalf("the confirm gate must surface a destructive reset:\n%s", view)
	}
	if !strings.Contains(view, "fleet-reset/") {
		t.Fatalf("it must say where the host's state is saved:\n%s", view)
	}
	repo := updplan.Default().Repos["dotfiles"]
	s, err := updexec.SyncScript(repo, updplan.LocalSkip, m2.ans.forceReset())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "reset --hard") {
		t.Fatal("the reset answer did not reach the sync step's script")
	}
}

// --- log timestamps -------------------------------------------------------

func TestLogLinesCarryAShortTimestampFirst(t *testing.T) {
	fixed := time.Date(2026, 8, 20, 14, 5, 9, 0, time.UTC)
	saved := nowFn
	nowFn = func() time.Time { return fixed }
	defer func() { nowFn = saved }()

	m := testModel("a")
	m.logOpen = true
	m.appendLog("a", "Installing sops...")
	line := ""
	for _, l := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.Contains(l, "Installing sops") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("log line not rendered")
	}
	ts, host := strings.Index(line, "14:05:09"), strings.Index(line, "a")
	if ts < 0 {
		t.Fatalf("expected a hh:mm:ss stamp: %q", line)
	}
	if ts > host {
		t.Fatalf("the timestamp must be the FIRST column: %q", line)
	}
}

// --- log navigation and search --------------------------------------------

func TestTabMovesTheVimKeysToTheLog(t *testing.T) {
	m := testModel("a", "b", "c")
	m.logOpen = true
	for i := 0; i < 40; i++ {
		m.appendLog("a", "line")
	}
	cursorBefore := m.cursor

	focused, _ := send(m, "tab")
	if !focused.logFocus {
		t.Fatal("tab must move focus to the log")
	}
	moved, _ := send(focused, "j")
	if moved.cursor != cursorBefore {
		t.Fatal("with the log focused, j must not move the host cursor")
	}
	if moved.logTop == 0 {
		t.Fatal("j should have scrolled the log")
	}
	// gg / G
	top, _ := send(moved, "g", "g")
	if top.logTop != 0 {
		t.Fatalf("gg must jump to the first log line, got %d", top.logTop)
	}
	end, _ := send(top, "G")
	if !end.logFollow {
		t.Fatal("G means 'show me the newest', i.e. resume following")
	}
	back, _ := send(end, "tab")
	if back.logFocus {
		t.Fatal("tab must return the keys to the host list")
	}
}

// Searching the log must not disturb a host filter the operator already set.
func TestLogSearchIsSeparateFromTheHostFilter(t *testing.T) {
	m := testModel("alpha", "beta")
	m.logOpen = true
	m.appendLog("alpha", "Installing sops")
	m.appendLog("alpha", "Building gss")

	hostFiltered, _ := send(m, "/", "a", "l", "enter") // host filter
	focused, _ := send(hostFiltered, "tab")
	logSearched, _ := send(focused, "/", "g", "s", "s", "enter")

	if logSearched.search.input != "al" {
		t.Fatalf("the host filter was clobbered: %q", logSearched.search.input)
	}
	if logSearched.logSearch.input != "gss" {
		t.Fatalf("the log pattern did not take: %q", logSearched.logSearch.input)
	}
	if len(logSearched.logMatches()) != 1 {
		t.Fatalf("expected one log match, got %d", len(logSearched.logMatches()))
	}
	// Jumping stops following, or the tail would yank the match off screen.
	if logSearched.logFollow {
		t.Fatal("jumping to a match must pause following")
	}
}

func TestLogSearchReportsWhenNothingMatches(t *testing.T) {
	m := testModel("a")
	m.logOpen = true
	m.appendLog("a", "hello")
	focused, _ := send(m, "tab")
	got, _ := send(focused, "/", "z", "z", "enter")
	if !strings.Contains(got.status, "no matches") {
		t.Fatalf("expected a no-match report, got %q", got.status)
	}
}
