package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLayout(t *testing.T) {
	const chrome = 7 // banner 5 + separator 1 + status 1, as measured at >=80 cols

	cases := []struct {
		name                 string
		vp, chrome           int
		p                    panes
		logActive, errActive bool
		want                 heights
	}{
		{
			name: "host only takes everything",
			vp:   40, chrome: chrome, p: panes{host: true},
			want: heights{host: 40 - chrome - panelFixedRows},
		},
		{
			name: "log only takes everything",
			vp:   40, chrome: chrome, p: panes{log: true}, logActive: true,
			want: heights{log: 40 - chrome - panelFixedRows},
		},
		{
			name: "err only takes everything",
			vp:   40, chrome: chrome, p: panes{err: true}, errActive: true,
			want: heights{err: 40 - chrome - panelFixedRows},
		},
		{
			name: "open but empty stream panes reserve no body rows",
			vp:   40, chrome: chrome, p: panes{host: true, log: true, err: true},
			want: heights{host: 40 - chrome - 3*panelFixedRows},
		},
		{
			name: "host plus log splits, host on top",
			vp:   40, chrome: chrome, p: panes{host: true, log: true}, logActive: true,
			want: heights{host: 8, log: 40 - chrome - 2*panelFixedRows - 8},
		},
		{
			name: "three panes split the bottom, log gets the odd row",
			vp:   40, chrome: chrome, p: panes{host: true, log: true, err: true},
			logActive: true, errActive: true,
			want: heights{host: 8, log: 8, err: 8},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := layout(c.vp, c.chrome, c.p, c.logActive, c.errActive)

			if got.host < 0 || got.log < 0 || got.err < 0 {
				t.Fatalf("negative height: %+v", got)
			}
			if !c.p.host && got.host != 0 {
				t.Fatalf("hidden host pane got %d rows", got.host)
			}
			if (!c.p.log || !c.logActive) && got.log != 0 {
				t.Fatalf("inactive log pane got %d rows", got.log)
			}
			if (!c.p.err || !c.errActive) && got.err != 0 {
				t.Fatalf("inactive error pane got %d rows", got.err)
			}
			// Bodies plus every open panel's fixed cost plus the chrome never
			// exceed the viewport. ONE definition of the total, shared with the
			// frame guard below.
			total := minFrameRows(c.chrome, c.p) + got.host + got.log + got.err
			if total > c.vp {
				t.Fatalf("layout overflows: %d > %d (%+v)", total, c.vp, got)
			}
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestThreePaneSplitIsEven(t *testing.T) {
	got := layout(40, 7, panes{host: true, log: true, err: true}, true, true)
	if got.log < got.err || got.log-got.err > 1 {
		t.Fatalf("bottom split must be even with the odd row to the log: %+v", got)
	}
	if got.err < minPaneRows {
		t.Fatalf("error pane below its floor at 40 rows: %+v", got)
	}
}

// The host table keeps the top fifth once a stream pane is flowing — the
// proportion the dashboard shipped with. Deliberately NOT today's exact
// number: today's overflows the terminal.
func TestHostPlusOneBottomPaneKeepsTheTopFifth(t *testing.T) {
	got := layout(40, 7, panes{host: true, log: true}, true, false)
	if got.err != 0 {
		t.Fatalf("a hidden error pane takes no rows: %+v", got)
	}
	if got.host != 40/hostShareDenom {
		t.Fatalf("host should keep the top fifth (%d), got %d", 40/hostShareDenom, got.host)
	}
	if got.log < minPaneRows {
		t.Fatalf("the log keeps its floor: %+v", got)
	}
}

// A viewport too small for the floors must still produce a frame that FITS.
// Floors are a preference: at 60x16 the chrome (8) plus three panel frames (3
// each) already claim 17 of the 16 rows.
func TestTinyViewportStillFits(t *testing.T) {
	const chrome = 8 // banner 6 at 60 cols + separator 1 + status 1
	p := panes{host: true, log: true, err: true}
	got := layout(16, chrome, p, true, true)

	if got != (heights{}) {
		t.Fatalf("with the fixed rows already over budget the panes take nothing: %+v", got)
	}
}

// The same rationing one pane up: at 80x12 with host+log the fixed rows are
// 7+3+3 = 13 for a 12-row terminal, so both panes yield.
func TestSmallTerminalYieldsRatherThanOverflowing(t *testing.T) {
	got := layout(12, 7, panes{host: true, log: true}, true, false)
	if got != (heights{}) {
		t.Fatalf("12 rows cannot hold two panels plus chrome: %+v", got)
	}
}

// TestFrameFitsTheTerminal is the invariant the dashboard never had: whatever
// the size, the mode, or which panes are open, the frame must fit. It FAILS on
// the code that preceded this work — the frame overflowed by up to 12 rows at
// 60x16.
//
// modeHelp is deliberately absent: its View() replaces the whole frame with
// the `?` overlay (no panes, no status), so it is not a pane-layout case, and
// the overlay's own height is a pre-existing limitation with its own
// follow-up.
func TestFrameFitsTheTerminal(t *testing.T) {
	sizes := [][2]int{{60, 16}, {80, 24}, {100, 40}, {200, 60}}
	combos := []panes{
		{host: true}, {log: true}, {err: true},
		{host: true, log: true}, {host: true, err: true},
		{log: true, err: true}, {host: true, log: true, err: true},
	}
	modes := []tuiMode{modeNormal, modeSearch, modeAnswers, modeConfirm}

	for _, size := range sizes {
		for _, p := range combos {
			for _, full := range []bool{false, true} {
				for _, mode := range modes {
					m := settledTestModel(8)
					m.vp = viewport{width: size[0], height: size[1]}
					m.hostOpen, m.logOpen, m.errOpen = p.host, p.log, p.err
					m.mode = mode
					if full {
						for i := 0; i < 200; i++ {
							m.appendLogLine("h1", fmt.Sprintf("line %d", i), i%3 == 0)
						}
					}
					v := m.View()

					// The achievable invariant: the PANES never add to an
					// over-budget frame. The budget is minFrameRows — the same
					// function layout() rations against — so the guard and the
					// layout cannot disagree.
					budget := size[1]
					if fixed := minFrameRows(m.chromeRows(), m.paneState()); fixed > budget {
						budget = fixed
					}
					if h := lipgloss.Height(v); h > budget {
						t.Errorf("%dx%d panes=%+v full=%v mode=%v: %d rows > %d",
							size[0], size[1], p, full, mode, h, budget)
					}
					if w := lipgloss.Width(v); w > size[0] {
						t.Errorf("%dx%d panes=%+v full=%v mode=%v: %d cols > %d",
							size[0], size[1], p, full, mode, w, size[0])
					}
					if strings.TrimSpace(v) == "" {
						t.Errorf("%dx%d panes=%+v: empty frame", size[0], size[1], p)
					}
				}
			}
		}
	}
}

// When the chrome alone is taller than the terminal — a dialog on a very short
// screen — every pane must yield rather than the layout piling rows onto a
// frame that already does not fit.
func TestChromeOverBudgetGivesThePanesNothing(t *testing.T) {
	m := settledTestModel(5)
	m.vp = viewport{width: 60, height: 16}
	m.hostOpen, m.logOpen, m.errOpen = true, true, true
	m.appendLogLine("h1", "out", false)
	m.appendLogLine("h1", "boom", true)
	m.mode = modeAnswers // the answer panel alone is 12 rows at 60 columns

	if minFrameRows(m.chromeRows(), m.paneState()) <= m.vp.height {
		t.Skip("the fixed rows fit here; this case only exists when they do not")
	}
	h := m.heights()
	if h.host != 0 || h.log != 0 || h.err != 0 {
		t.Fatalf("over-budget chrome must leave the panes nothing, got %+v", h)
	}
}

// A body row must be exactly ONE rendered line, or every height in layout() is
// meaningless. th.panel has Padding(0,1), so panel.Width(w) leaves w-2 usable
// columns (measured: at Width(76) a 74-char line renders 3 rows and a 75-char
// line renders 4) — but View() used to truncate rows to panelWidth() and the
// column header not at all. At 80 columns an 8-host panel rendered 20 lines.
func TestPanelBodyRowIsExactlyOneLine(t *testing.T) {
	for _, w := range []int{60, 80, 100, 200} {
		m := settledTestModel(8)
		m.vp = viewport{width: w, height: 60} // tall enough that nothing is cut
		m.logOpen, m.errOpen = false, false
		// The widest row this table can produce, so the check is not passing
		// only because the fixture is short.
		m.rows[0].Alias = "host-with-a-very-long-name-xyz"
		m.rows[0].Branch = "feature/a-long-branch-name"
		m.rows[0].InstalledBranch = "main"
		m.updating[m.rows[0].Alias] = updState{
			phase: updFail,
			log:   "install.sh: a very long failure explanation that must be truncated, not wrapped",
		}

		want := len(m.rows) + panelFixedRows
		if got := lipgloss.Height(m.View()) - m.chromeRows(); got != want {
			t.Errorf("width %d: host panel rendered %d lines for %d rows, want %d",
				w, got, len(m.rows), want)
		}
	}
}

// An empty fleet must not swallow the other panes: View() used to return early
// from inside the "no fleet hosts found" branch.
func TestEmptyFleetStillRendersTheOtherPanes(t *testing.T) {
	m := testModel() // no hosts
	m.vp = viewport{width: 100, height: 40}
	m.hostOpen, m.errOpen = false, true
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	v := m.View()
	if !strings.Contains(v, "apt-get") {
		t.Fatal("an empty fleet must not swallow the error pane")
	}
	if lipgloss.Height(v) > 40 {
		t.Fatalf("empty fleet frame overflows: %d rows", lipgloss.Height(v))
	}
}

func TestFrameFitsWithZeroAndOneHost(t *testing.T) {
	for _, n := range []int{0, 1} {
		m := settledTestModel(n)
		m.vp = viewport{width: 60, height: 16}
		m.hostOpen, m.logOpen, m.errOpen = true, true, true
		budget := 16
		if fixed := minFrameRows(m.chromeRows(), m.paneState()); fixed > budget {
			budget = fixed
		}
		if h := lipgloss.Height(m.View()); h > budget {
			t.Fatalf("%d hosts: %d rows > %d", n, h, budget)
		}
	}
}
