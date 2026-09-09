package cmd

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// scrolledFullModel fills the log ring to exactly logCap with the line that
// the NEXT append will evict placed at the FRONT (frontStderr or not). It also
// guarantees at least six stderr entries so the model can sit scrolled at
// errTop=5 (a real index into the filtered stderr view).
func scrolledFullModel(t *testing.T, frontStderr bool) tuiModel {
	t.Helper()
	m := newTUIModel(hosts("h"), nil, fakeBaseline{head: "x"}, testNow, "main", 2, updplan.Default())
	m.errOpen = true
	m.errFollow = false
	// The line to be evicted is the OLDEST, so it is appended first.
	m.appendLogLine("h", "front", frontStderr)
	// Six stderr lines give the filtered view a real depth for errTop=5.
	for i := 0; i < 6; i++ {
		m.appendLogLine("h", "stderr", true)
	}
	// Top the ring up to logCap with stdout filler.
	for len(m.logs) < logCap {
		m.appendLogLine("h", "fill", false)
	}
	if len(m.logs) != logCap {
		t.Fatalf("setup: len(logs)=%d, want %d", len(m.logs), logCap)
	}
	if len(m.errEntries()) < 6 {
		t.Fatalf("setup: errEntries=%d, want >=6", len(m.errEntries()))
	}
	m.errTop = 5
	return m
}

func TestEvictingAStdoutLineLeavesErrTopAlone(t *testing.T) {
	// Front is a STDOUT line. Evicting it must not move the filtered stderr
	// view, so errTop stays put.
	m := scrolledFullModel(t, false)
	want := m.errTop
	m.appendLogLine("h", "newest", false)
	if got := m.errTop; got != want {
		t.Fatalf("evicted line was stdout; the stderr view is unchanged but errTop moved %d -> %d", want, got)
	}
}

func TestEvictingAStderrLineShiftsErrTop(t *testing.T) {
	// Front is a STDERR line. Evicting it removes one entry from the filtered
	// view, so errTop moves down by one.
	m := scrolledFullModel(t, true)
	want := m.errTop
	m.appendLogLine("h", "newest", false)
	if got := m.errTop; got != want-1 {
		t.Fatalf("errTop should shift by one on a stderr eviction: %d -> %d", want, got)
	}
}

func TestEvictionKeepsErrCountHonest(t *testing.T) {
	// After eviction the stderr counter must still equal the filtered view.
	m := scrolledFullModel(t, true) // front is stderr
	for i := 0; i < 5; i++ {
		m.appendLogLine("h", "newest", false)
	}
	if got, want := m.errCount, len(m.errEntries()); got != want {
		t.Fatalf("errCount=%d, but the filtered view has %d entries", got, want)
	}
}
