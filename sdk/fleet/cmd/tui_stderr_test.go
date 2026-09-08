package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func TestStderrLineIsTaggedAndCounted(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "installing", false)
	m.appendLogLine("a", "WARNING: apt-get update failed", true)
	m.appendLogLine("a", "Receiving objects:  73% (30/41)", true) // benign

	if len(m.logs) != 3 {
		t.Fatalf("the log buffer keeps every line, got %d", len(m.logs))
	}
	if got := m.errEntries(); len(got) != 2 {
		t.Fatalf("the error projection is the stderr subset, got %d", len(got))
	}
	if m.warns["a"] != 1 {
		t.Fatalf("only non-benign stderr raises the warning count, got %d", m.warns["a"])
	}
}

// appendLog (the 2-arg form every existing test uses) must keep meaning stdout.
func TestAppendLogStaysStdout(t *testing.T) {
	m := testModel("a")
	m.appendLog("a", "hello")
	if m.logs[0].stderr || m.warns["a"] != 0 {
		t.Fatal("appendLog must remain the stdout form")
	}
}

// errCount is what errActive() reads on every keystroke and spinner tick, so
// it must not drift from the projection when the cap evicts tagged entries.
func TestErrCountTracksTheProjection(t *testing.T) {
	m := testModel("a")
	for i := 0; i < logCap+50; i++ {
		m.appendLogLine("a", fmt.Sprintf("line %d", i), i%2 == 0)
	}
	if m.errCount != len(m.errEntries()) {
		t.Fatalf("errCount drifted from the projection after eviction: %d != %d",
			m.errCount, len(m.errEntries()))
	}
}

// The whole wire in one test: a Fake host writes to stderr, the real
// Executor/Console/Background lane carries it, and the model ends with the
// line in BOTH panes and a warning on the row.
func TestStderrReachesBothPanesAndTheBadge(t *testing.T) {
	f := runner.Fake{
		Out:    map[string]string{"h1": "installing"},
		ErrOut: map[string]string{"h1": "WARNING: apt-get update failed"},
	}
	m := settledTestModel(1)
	m.run = f
	m.vp = viewport{width: 120, height: 40}
	m.errOpen = true

	cmd := beginStream("h1", updplan.Default(), answers{}, f, t.TempDir())
	msg := cmd()
	st := msg.(streamStartedMsg).st

	for {
		lm := readLine("h1", st)()
		if _, done := lm.(logEOFMsg); done {
			break
		}
		l := lm.(logLineMsg)
		m.appendLogLine(l.alias, l.line, l.stderr)
	}

	// updplan.Default() has TWO steps and the Fake replays Out/ErrOut for each,
	// so assert the relationships rather than absolute counts.
	errs := m.errEntries()
	if len(errs) == 0 {
		t.Fatal("the error pane must have the stderr line")
	}
	if m.warns["h1"] != len(errs) {
		t.Fatalf("every non-benign stderr line is a warning: warns=%d errEntries=%d",
			m.warns["h1"], len(errs))
	}
	for _, e := range errs {
		if !strings.Contains(e.line, "apt-get") {
			t.Fatalf("the error projection must hold only the stderr line, got %q", e.line)
		}
	}
	if len(m.logs) != 2*len(errs) {
		t.Fatalf("the log pane keeps BOTH streams (one stdout per stderr here), got %d for %d",
			len(m.logs), len(errs))
	}
	v := m.View()
	if !strings.Contains(v, "installing") || !strings.Contains(v, "apt-get") {
		t.Fatal("both lines must be on screen")
	}
}

func TestOkWithWarningsBadge(t *testing.T) {
	m := settledTestModel(2)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h1", "WARNING: grouped install failed", true)
	m.appendLogLine("h2", "Receiving objects:  73% (30/41)", true) // benign only
	m.updating["h1"] = updState{phase: updOK}
	m.updating["h2"] = updState{phase: updOK}

	if got := m.updateCell("h1"); !strings.Contains(got, "ok") || !strings.Contains(got, "2") {
		t.Fatalf("an exit-0 run that wrote stderr must show ok + the count, got %q", got)
	}
	if got := m.updateCell("h2"); strings.Contains(got, "⚠") {
		t.Fatalf("benign-only stderr must not raise a warning, got %q", got)
	}
}

func TestFailRowIsNeverAWarning(t *testing.T) {
	m := settledTestModel(1)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.updating["h1"] = updState{phase: updFail, log: "install.sh exited 1"}
	got := m.updateCell("h1")
	if !strings.Contains(got, "FAIL") {
		t.Fatalf("a failure still reads FAIL, got %q", got)
	}
	if strings.Contains(got, "ok") {
		t.Fatalf("a failed row must never read ok, got %q", got)
	}
}

func TestStatusBarWarningSummary(t *testing.T) {
	m := settledTestModel(2)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h2", "WARNING: grouped install failed", true)
	if got := m.statusView(); !strings.Contains(got, "⚠") || !strings.Contains(got, "2") {
		t.Fatalf("status bar must summarise warnings, got %q", got)
	}
}

func TestNewAttemptClearsTheWarningBadge(t *testing.T) {
	m := settledTestModel(2)
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h2", "WARNING: grouped install failed", true)
	m.startUpdate([]string{"h1"})
	if m.warns["h1"] != 0 {
		t.Fatalf("a new attempt starts with a clean badge, got %d", m.warns["h1"])
	}
	if m.warns["h2"] != 1 {
		t.Fatalf("another host's badge must be untouched, got %d", m.warns["h2"])
	}
}
