package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestDemoFrames renders the dashboard's real states through the real model
// and View(), and with FLEET_DEMO=1 prints them for a human to look at:
//
//	go test ./cmd/ -run TestDemoFrames -v            # assert (ASCII, CI-safe)
//	FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames  # watch (colour)
//
// It doubles as the golden-frame gate: every state must render, stay inside
// the terminal width, and contain its defining marker.
func TestDemoFrames(t *testing.T) {
	demo := os.Getenv("FLEET_DEMO") == "1"
	if demo {
		lipgloss.SetColorProfile(termenv.ANSI256)
		defer lipgloss.SetColorProfile(termenv.Ascii)
	}

	base := fakeBaseline{head: "72392c9"}
	build := func() tuiModel {
		m := newTUIModel(hosts("host-desktop", "host-nano", "host-pi", "host-edge", "host-lab"),
			nil, base, testNow, "main", 2)
		m.vp = viewport{height: 16, width: 100}
		return m
	}

	// A realistic mixed fleet: one of every status class.
	resolved := []Row{
		{Alias: "host-desktop", Class: "up-to-date", Commit: "72392c9", Age: testNow.Add(-2 * time.Hour)},
		{Alias: "host-nano", Class: "behind", Behind: 24, Commit: "9484943", Age: testNow.Add(-16 * 24 * time.Hour)},
		{Alias: "host-pi", Class: "unreachable"},
		{Alias: "host-edge", Class: "unknown", Note: "corrupt stamp"},
		{Alias: "host-lab", Class: "divergent", Commit: "abc1234", Age: testNow.Add(-3 * time.Hour)},
	}
	settled := func() tuiModel {
		m := build()
		for _, r := range resolved {
			m.setRow(r)
			delete(m.pending, r.Alias)
		}
		m.resort()
		return m
	}

	frames := []struct {
		name, must string
		build      func() tuiModel
	}{
		{"1. opening — every host still polling (no blocking collect)", "polling", build},
		{"2. streamed in — worst-first, status-coloured", "unreachable", settled},
		{"3. search: /host-[np] — smartcase, live match count", "match(es)", func() tuiModel {
			m, _ := send(settled(), "/", "h", "o", "s", "t", "-", "[", "n", "p", "]")
			return m
		}},
		{"4. invalid regex — inline error, still editable", "bad pattern", func() tuiModel {
			m, _ := send(settled(), "/", "[")
			return m
		}},
		{"5. visual selection (v + j) — 2 hosts marked", "selected", func() tuiModel {
			m, _ := send(settled(), "v", "j")
			return m
		}},
		{"6. confirm strip — targets listed before anything runs", "y: go", func() tuiModel {
			m, _ := send(settled(), "v", "j", " ", "u")
			return m
		}},
		{"7. CONCURRENT UPDATE — 2 running at once (jobs=2), 1 queued", "updating", func() tuiModel {
			m, _ := send(settled(), "v", "j", " ", "u")
			m, _ = send(m, "y")
			var mm = m
			for _, a := range []string{"host-pi", "host-nano", "host-edge"} {
				x, _ := mm.Update(precheckMsg{alias: a, interactive: false})
				mm = x.(tuiModel)
			}
			return mm
		}},
		{"8. mid-wave: one FAILED with its cause, slot refilled, UI still live", "FAIL", func() tuiModel {
			m := settled()
			m.jobs = 2
			m.updating["host-pi"] = updState{phase: updRunning}
			m.running = 2
			m.updating["host-nano"] = updState{phase: updRunning}
			m.bgQueue = []string{"host-edge"}
			x, _ := m.Update(bgUpdateDoneMsg{alias: "host-pi",
				log: "fatal: could not read Username for 'https://github.com'", err: fmt.Errorf("exit status 128")})
			return x.(tuiModel)
		}},
		{"9. interactive fallback — host needing sudo waits its turn", "queued", func() tuiModel {
			m := settled()
			m.updating["host-nano"] = updState{phase: updRunning}
			m.running = 1
			x, _ := m.Update(precheckMsg{alias: "host-pi", interactive: true})
			return x.(tuiModel)
		}},
		{"10. help overlay", "toggle this help", func() tuiModel {
			m, _ := send(settled(), "?")
			return m
		}},
		{"11. empty fleet — points at discover/add", "fleet discover", build2Empty},
	}

	for _, f := range frames {
		m := f.build()
		out := m.View()
		if !strings.Contains(out, f.must) {
			t.Errorf("frame %q missing %q:\n%s", f.name, f.must, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > m.vp.width {
				t.Errorf("frame %q line too wide (%d): %q", f.name, w, line)
			}
		}
		if demo {
			fmt.Printf("\n\033[1;33m━━━ %s ━━━\033[0m\n%s\n", f.name, out)
		}
	}
}

func build2Empty() tuiModel {
	m := newTUIModel(nil, nil, fakeBaseline{head: "72392c9"}, testNow, "main", 2)
	m.vp = viewport{height: 16, width: 100}
	return m
}
