package app

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/pane"
)

// Deck tests run small shell programs that print a marker and then wait.
func marker(s string) string { return "sh -c 'echo " + s + "; exec cat'" }

func deckConfig(zoomB bool) config.Config {
	f := false
	return config.Config{
		Prefix: "ctrl+b",
		Title:  config.Title{Size: "5%"},
		Panes: []config.Pane{
			{Name: "top", Size: "60%", Cmd: marker("TOP-PANE"), Probe: "none"},
			{Name: "mid", Size: "30%", Tools: []config.Tool{
				{Name: "a", Views: []config.View{{Name: "a1", Cmd: marker("VIEW-A1"), Probe: "none"}}},
				{Name: "b", Zoom: zoomB, Views: []config.View{
					{Name: "b1", Cmd: marker("VIEW-B1"), Probe: "none"},
					{Name: "b2", Cmd: marker("VIEW-B2"), Probe: "none"},
				}},
			}},
			{Name: "bot", Size: "5%", Cmd: "echo status-line", Every: "200ms", Probe: "none", Zoomable: &f},
		},
	}
}

// clock is a settable time source for double-esc tests.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func startDeck(t *testing.T, cfg config.Config) (*App, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	a, err := New(Options{Config: cfg, Env: env{}, Title: "mono vtest", ConfigPath: "/nonexistent/config.json", Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 40)
	t.Cleanup(a.Stop)
	return a, clk
}

const esc = "\x1b"

func enterNav(a *App, clk *clock) {
	a.HandleBytes([]byte(esc))
	clk.add(100 * time.Millisecond)
	a.HandleBytes([]byte(esc))
}

func focusMid(t *testing.T, a *App) {
	t.Helper()
	if err := a.focusNamed([]string{"mid"}); err != nil {
		t.Fatal(err)
	}
}

// E18: only the visible view's program runs; switching stops the old one.
func TestOnlyVisibleViewRuns(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	eventually(t, a, "view a1", func(s string) bool { return strings.Contains(s, "VIEW-A1") })
	if got := strings.Join(a.Running(), ","); got != "top,mid/a/a1,bot" {
		t.Fatalf("running = %s", got)
	}
	focusMid(t, a)
	enterNav(a, clk)
	a.HandleBytes([]byte("l"))
	eventually(t, a, "view b1", func(s string) bool { return strings.Contains(s, "VIEW-B1") && !strings.Contains(s, "VIEW-A1") })
	if got := strings.Join(a.Running(), ","); got != "top,mid/b/b1,bot" {
		t.Fatalf("after l, running = %s", got)
	}
	a.HandleBytes([]byte("j"))
	eventually(t, a, "view b2", func(s string) bool { return strings.Contains(s, "VIEW-B2") })
	if got := strings.Join(a.Running(), ","); got != "top,mid/b/b2,bot" {
		t.Fatalf("after j, running = %s", got)
	}
}

// The title names where you are: pane, tool with arrows, view i/n, NAV.
func TestTitleLabel(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	focusMid(t, a)
	s := eventually(t, a, "label", func(s string) bool { return strings.Contains(s, "mid a ▶ · a1") })
	if strings.Contains(s, "NAV") {
		t.Fatalf("NAV before nav mode:\n%s", s)
	}
	enterNav(a, clk)
	a.HandleBytes([]byte("l"))
	a.HandleBytes([]byte("j"))
	eventually(t, a, "label after moves", func(s string) bool { return strings.Contains(s, "mid ◀ b · b2 2/2") && strings.Contains(s, "NAV") })
	t.Setenv("FLEET_ASCII_BORDERS", "1")
	eventually(t, a, "ascii label", func(s string) bool { return strings.Contains(s, "mid < b · b2 2/2") })
}

// E19: :focus pane/tool/view jumps there; a bad name lists the valid ones.
func TestFocusCommand(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"mid/b/b2"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, a, "b2 via :focus", func(s string) bool { return strings.Contains(s, "VIEW-B2") })
	err := a.focusNamed([]string{"mid/zz"})
	if err == nil || !strings.Contains(err.Error(), "a, b") {
		t.Fatalf("bad tool: %v", err)
	}
	err = a.focusNamed([]string{"mid/b/nope"})
	if err == nil || !strings.Contains(err.Error(), "b1, b2") {
		t.Fatalf("bad view: %v", err)
	}
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
}

// E20: entering nav mode — prefix + nav key, or esc esc inside the window;
// the first esc is forwarded at once; nav_esc_ms 0 turns double-esc off.
func TestNavEntry(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	enterNav(a, clk)
	if !a.Nav() {
		t.Fatal("esc esc within 100 ms did not enter nav mode")
	}
	a.HandleBytes([]byte(esc))
	if a.Nav() {
		t.Fatal("esc did not leave nav mode")
	}
	a.HandleBytes([]byte(esc))
	clk.add(400 * time.Millisecond)
	a.HandleBytes([]byte(esc))
	if a.Nav() {
		t.Fatal("esc esc 400 ms apart entered nav mode (window is 300 ms)")
	}
	a.HandleBytes([]byte{0x02, ' '}) // ctrl+b space
	if !a.Nav() {
		t.Fatal("prefix + space did not enter nav mode")
	}
	a.HandleBytes([]byte(esc))

	cfg := deckConfig(false)
	zero := 0
	cfg.NavEscMs = &zero
	b, clk2 := startDeck(t, cfg)
	enterNav(b, clk2)
	if b.Nav() {
		t.Fatal("nav_esc_ms 0 must turn double-esc off")
	}
}

// E21: j/k walk a deck's views, then on to the panes; h/l switch tools; the
// ends do not wrap.
func TestNavWalk(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	enterNav(a, clk)
	a.HandleBytes([]byte("k")) // already at the first pane: no wrap
	if a.FocusedName() != "top" {
		t.Fatalf("k at the top wrapped to %s", a.FocusedName())
	}
	a.HandleBytes([]byte("j"))
	if a.FocusedName() != "mid" {
		t.Fatalf("j from top → %s", a.FocusedName())
	}
	a.HandleBytes([]byte("h")) // first tool: no wrap
	a.HandleBytes([]byte("l"))
	a.HandleBytes([]byte("l")) // last tool: stays
	a.HandleBytes([]byte("j")) // b1 → b2
	if got := a.FocusedPath(); got != "mid/b/b2" {
		t.Fatalf("path = %s", got)
	}
	a.HandleBytes([]byte("j")) // last view, and bot is fixed: stays
	if got := a.FocusedPath(); got != "mid/b/b2" {
		t.Fatalf("j past the last view → %s (a fixed pane is not a nav stop)", got)
	}
	a.HandleBytes([]byte("k"))
	if got := a.FocusedPath(); got != "mid/b/b1" {
		t.Fatalf("k back through the deck → %s", got)
	}
}

// prefix j/k cycle the middle panes only: a fixed pane (zoomable: false, the
// gsl footer) is never a stop, but :focus still reaches it by name.
func TestCycleSkipsFixedPanes(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for i := 0; i < 4; i++ {
		a.HandleBytes([]byte{0x02, 'j'})
		seen = append(seen, a.FocusedName())
	}
	if got := strings.Join(seen, " "); got != "mid top mid top" {
		t.Fatalf("ctrl+b j visits %s", got)
	}
	a.HandleBytes([]byte{0x02, 'k'})
	if a.FocusedName() != "mid" {
		t.Fatalf("ctrl+b k → %s", a.FocusedName())
	}
	if err := a.focusNamed([]string{"bot"}); err != nil || a.FocusedName() != "bot" {
		t.Fatalf(":focus bot → %s, %v", a.FocusedName(), err)
	}
}

// E22: any other key leaves nav mode and still reaches the program, so a
// vim user's "esc esc :wq" loses nothing (sdk/mono AGENTS: mono never eats
// a key outside the prefix).
func TestNavOtherKeyLeaves(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	enterNav(a, clk)
	a.HandleBytes([]byte("x"))
	if a.Nav() {
		t.Fatal("x did not leave nav mode")
	}
	a.HandleBytes([]byte("Q\r"))
	eventually(t, a, "the nav-leaving x and the typed Q echoed by cat", func(s string) bool { return strings.Contains(s, "xQ") })
}

// The prefix in nav mode leaves nav mode and starts a command: ctrl+b q
// quits mono rather than typing q into the program.
func TestNavPrefixRunsCommand(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	enterNav(a, clk)
	a.HandleBytes([]byte{0x02})
	a.HandleBytes([]byte("q"))
	if !a.Quit() {
		t.Fatal("ctrl+b q in nav mode did not quit")
	}
}

// One read holding "esc esc ctrl+b z": nav mode begins and the prefix key
// still runs as a command (zoom), not dropped with the router left waiting.
func TestNavBurstKeepsPrefixKey(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	before := a.Heights()
	a.HandleBytes([]byte("\x1b\x1b\x02z"))
	if !a.Nav() {
		t.Fatal("esc esc did not enter nav mode")
	}
	if z := a.Heights(); z[2] != 0 || z[1] == before[1] {
		t.Fatalf("ctrl+b z in the burst did not zoom: %v → %v", before, z)
	}
	// A prefix that ends the read waits for its key across reads.
	b, _ := startDeck(t, deckConfig(false))
	b.HandleBytes([]byte("\x1b\x1b\x02"))
	b.HandleBytes([]byte("q"))
	if !b.Quit() {
		t.Fatal("esc esc ctrl+b | q did not quit")
	}
}

// Both escapes reach the program: double-esc only adds nav mode.
func TestDoubleEscForwardsBoth(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	var got []byte
	fwd1, e1 := a.doubleEsc([]byte(esc))
	clk.add(50 * time.Millisecond)
	fwd2, e2 := a.doubleEsc([]byte(esc))
	got = append(append(got, fwd1...), fwd2...)
	if string(got) != esc+esc || e1 || !e2 {
		t.Fatalf("forwarded %q, entered %v/%v", got, e1, e2)
	}
	if fwd, e := a.doubleEsc([]byte(esc + esc)); string(fwd) != esc+esc || !e {
		t.Fatalf("one-read esc esc forwarded %q, entered %v", fwd, e)
	}
}

// E25: z zooms the focused middle pane over every middle row; the title and
// the non-zoomable footer keep theirs; z again restores the split.
func TestZoomKeepsTitleAndFooter(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	before := a.Heights()
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	a.HandleBytes([]byte{0x02, 'z'})
	z := a.Heights()
	if z[0] != before[0] || z[3] != before[3] {
		t.Fatalf("title/footer moved: %v → %v", before, z)
	}
	if z[1] != before[1]+before[2] || z[2] != 0 {
		t.Fatalf("zoom rows: %v → %v", before, z)
	}
	s := eventually(t, a, "ZOOM on the title", func(s string) bool { return strings.Contains(s, "ZOOM") })
	if strings.Contains(s, "VIEW-A1") {
		t.Fatalf("the hidden pane is still drawn:\n%s", s)
	}
	a.HandleBytes([]byte{0x02, 'z'})
	if got := a.Heights(); got[1] != before[1] || got[2] != before[2] {
		t.Fatalf("unzoom: %v, want %v", got, before)
	}
	if err := a.focusNamed([]string{"bot"}); err != nil {
		t.Fatal(err)
	}
	a.HandleBytes([]byte{0x02, 'z'})
	if got := a.Heights(); got[1] != before[1] {
		t.Fatalf("z on the footer changed the layout: %v", got)
	}
	eventually(t, a, "a message about the footer", func(s string) bool { return strings.Contains(s, "not part of the middle") })
}

// E26: a zoom: true tool fills the middle while on screen, and gives the
// rows back when you switch away.
func TestToolZoom(t *testing.T) {
	a, clk := startDeck(t, deckConfig(true))
	before := a.Heights()
	focusMid(t, a)
	enterNav(a, clk)
	a.HandleBytes([]byte("l")) // to tool b (zoom: true)
	z := a.Heights()
	if z[2] != before[1]+before[2] || z[1] != 0 || z[0] != before[0] || z[3] != before[3] {
		t.Fatalf("tool zoom: %v → %v", before, z)
	}
	a.HandleBytes([]byte("h")) // back to tool a
	if got := a.Heights(); got[1] != before[1] || got[2] != before[2] {
		t.Fatalf("switching away did not restore: %v, want %v", got, before)
	}
}

// E27 (runtime): middle: single runs one deck holding every middle pane's
// tools; the footer is untouched.
func TestMiddleSingleRuntime(t *testing.T) {
	cfg := deckConfig(false)
	cfg.Middle = "single"
	a, _ := startDeck(t, cfg)
	if got := strings.Join(a.PaneNames(), ","); got != "mid,bot" {
		t.Fatalf("panes = %s", got)
	}
	if got := strings.Join(a.Running(), ","); got != "mid/top/top,bot" {
		t.Fatalf("running = %s", got)
	}
}

// Review finding 2: :focus on a pane hidden by zoom makes it visible
// instead of focusing something you cannot see.
func TestFocusUnhidesAZoomedOutPane(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	a.HandleBytes([]byte{0x02, 'z'}) // zoom top: mid gets 0 rows
	if h := a.Heights(); h[2] != 0 {
		t.Fatalf("setup: mid not hidden: %v", h)
	}
	if err := a.focusNamed([]string{"mid"}); err != nil {
		t.Fatal(err)
	}
	h := a.Heights()
	if a.FocusedName() != "mid" || h[2] == 0 {
		t.Fatalf("focus %s with mid rows %d (heights %v): the focused pane must be visible", a.FocusedName(), h[2], h)
	}
}

// Review finding 3: programs a view switch stopped, and panes a reload
// started, never outlive Stop.
func TestStopWaitsForSwitchedAwayPrograms(t *testing.T) {
	cfg := deckConfig(false)
	// a1 ignores SIGHUP, so only the SIGKILL after KillGrace ends it.
	cfg.Panes[1].Tools[0].Views[0].Cmd = "sh -c 'trap \"\" HUP; echo VIEW-A1; while :; do sleep 1; done'"
	a, clk := startDeck(t, cfg)
	eventually(t, a, "a1", func(s string) bool { return strings.Contains(s, "VIEW-A1") })
	old := a.paneFor(t, "mid")
	focusMid(t, a)
	enterNav(a, clk)
	a.HandleBytes([]byte("l")) // switch away: a1 is being stopped in the background
	a.Stop()
	// The kill has been sent once Stop returns; the exit is recorded by the
	// pane's waiter a moment later. Without the fix the kill only comes after
	// KillGrace (500 ms), so 100 ms tells the two apart.
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		if exited, _ := old.Exited(); exited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Stop returned while a switched-away program was still running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNoPanesStartAfterStop(t *testing.T) {
	a, _ := startDeck(t, deckConfig(false))
	before := strings.Join(a.Running(), ",")
	a.Stop()
	a.applyMu.Lock()
	a.apply(deckConfig(true)) // a reload landing after quit
	a.applyMu.Unlock()
	if got := strings.Join(a.Running(), ","); got != before {
		t.Fatalf("apply after Stop changed the panes: %s → %s", before, got)
	}
}

// Review finding 4: an arrow key split across two reads in nav mode is an
// arrow, not esc (which would leave nav mode and leak "[A" to the program).
func TestNavSplitEscapeSequence(t *testing.T) {
	a, clk := startDeck(t, deckConfig(false))
	if err := a.focusNamed([]string{"top"}); err != nil {
		t.Fatal(err)
	}
	enterNav(a, clk)
	a.HandleBytes([]byte("\x1b["))
	a.HandleBytes([]byte("B")) // down
	if !a.Nav() || a.FocusedName() != "mid" {
		t.Fatalf("split down-arrow: nav %v, focus %s", a.Nav(), a.FocusedName())
	}
}

// paneFor is the running pane of a slot (tests only).
func (a *App) paneFor(t *testing.T, name string) *pane.Pane {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.slots {
		if s.name == name {
			return s.p
		}
	}
	t.Fatalf("no slot %s", name)
	return nil
}

// prefix r on a deck re-checks its tools against the running config; it does
// not re-read the config file (an unsaved or broken edit is not applied).
func TestDeckRestartRechecksWithoutReload(t *testing.T) {
	loaded := false
	var mu sync.Mutex
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	a, err := New(Options{Config: deckConfig(false), Env: env{}, Title: "mono vtest", ConfigPath: "/nonexistent/config.json", Now: clk.now,
		Load: func(string) (config.Config, error) {
			mu.Lock()
			loaded = true
			mu.Unlock()
			return config.Config{}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 40)
	t.Cleanup(a.Stop)
	focusMid(t, a)
	a.HandleBytes([]byte{0x02, 'r'})
	eventually(t, a, "re-check message", func(s string) bool { return strings.Contains(s, "re-checked tools") })
	eventually(t, a, "deck still running", func(s string) bool { return strings.Contains(s, "VIEW-A1") })
	mu.Lock()
	defer mu.Unlock()
	if loaded {
		t.Fatal("prefix r on a deck re-read the config file")
	}
}
