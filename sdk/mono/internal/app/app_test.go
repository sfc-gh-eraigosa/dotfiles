package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

// Panes here run cat or sh, never this test binary.

type env struct{ missing map[string]bool }

func (e env) LookPath(b string) (string, error) {
	if e.missing[b] {
		return "", errors.New("missing")
	}
	return "/bin/" + b, nil
}
func (env) Run(context.Context, string) bool { return true }
func (env) DRMDrivers() []string             { return nil }

func testConfig() config.Config {
	return config.Config{
		Prefix: "ctrl+b",
		Title:  config.Title{Size: "5%"},
		Panes: []config.Pane{
			{Name: "top", Size: "60%", Cmd: "cat", Probe: "none"},
			{Name: "mid", Size: "30%", Cmd: "cat", Probe: "none"},
			{Name: "bot", Size: "5%", Cmd: "echo status-line", Every: "200ms", Probe: "none"},
		},
	}
}

func start(t *testing.T, cfg config.Config, e env) *App {
	t.Helper()
	a, err := New(Options{Config: cfg, Env: e, Title: "mono vtest — abc1234", ConfigPath: "/nonexistent/config.json"})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 40)
	t.Cleanup(a.Stop)
	return a
}

func plain(rows []string) string {
	var b strings.Builder
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if r[i] == 0x1b {
				for i < len(r) && !(r[i] >= 0x40 && r[i] <= 0x7e && r[i] != '[') {
					i++
				}
				continue
			}
			b.WriteByte(r[i])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func eventually(t *testing.T, a *App, what string, cond func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var s string
	for time.Now().Before(deadline) {
		rows, _ := a.Frame()
		s = plain(rows)
		if cond(s) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; screen:\n%s", what, s)
	return s
}

func TestFrameSizeAndLayout(t *testing.T) {
	a := start(t, testConfig(), env{})
	rows, _ := a.Frame()
	if len(rows) != 40 {
		t.Fatalf("frame has %d rows, want 40", len(rows))
	}
	for i, r := range rows {
		if w := width(r); w != 60 {
			t.Fatalf("row %d is %d columns wide, want 60: %q", i, w, r)
		}
	}
	s := eventually(t, a, "every pane output", func(s string) bool { return strings.Contains(s, "status-line") })
	lines := strings.Split(s, "\n")
	// title 2 rows, top 24, mid 12, bot 2 at 40 rows: gsl-style output starts row 38.
	if !strings.Contains(lines[0], "mono vtest — abc1234") || !strings.Contains(lines[0], "● top") || !strings.Contains(lines[0], "ctrl+b ? help") {
		t.Fatalf("title bar = %q", lines[0])
	}
	if !strings.Contains(lines[38], "status-line") {
		t.Fatalf("every pane not on row 38:\n%s", s)
	}
}

func TestKeysGoToFocusedPaneAndPrefixMovesFocus(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("alpha\r"))
	s := eventually(t, a, "alpha in top pane", func(s string) bool { return strings.Contains(s, "alpha") })
	if i := strings.Index(s, "alpha"); strings.Count(s[:i], "\n") > 25 {
		t.Fatalf("alpha not in the top pane:\n%s", s)
	}
	a.HandleBytes([]byte("\x02j"))
	a.HandleBytes([]byte("bravo\r"))
	s = eventually(t, a, "bravo in mid pane", func(s string) bool { return strings.Contains(s, "bravo") })
	if row := strings.Count(s[:strings.Index(s, "bravo")], "\n"); row < 26 || row > 37 {
		t.Fatalf("bravo on row %d, want the mid pane (26-37):\n%s", row, s)
	}
	if !strings.Contains(strings.Split(s, "\n")[0], "● mid") {
		t.Fatal("title does not show the focused pane")
	}
	a.HandleBytes([]byte("\x02k\x02k")) // wraps: mid -> top -> bot
	if got := strings.Split(plain(first(a)), "\n")[0]; !strings.Contains(got, "● bot") {
		t.Fatalf("focus did not wrap to the bottom pane: %q", got)
	}
}

func first(a *App) []string { r, _ := a.Frame(); return r }

func TestPrefixPendingShowsHint(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02"))
	bar := strings.Split(plain(first(a)), "\n")[0]
	for _, want := range []string{"ctrl+b:", "j/k pane", ": command", "? help", "r restart", "q quit"} {
		if !strings.Contains(bar, want) {
			t.Errorf("pending hint %q missing %q", bar, want)
		}
	}
	a.HandleBytes([]byte("\x1b")) // esc cancels
	if bar := strings.Split(plain(first(a)), "\n")[0]; strings.Contains(bar, "ctrl+b:") {
		t.Fatalf("esc did not cancel the prefix: %q", bar)
	}
}

func TestLiteralPrefixReachesProgram(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02\x02x\r"))
	eventually(t, a, "^B echoed by cat", func(s string) bool { return strings.Contains(s, "^Bx") })
}

func TestHelpOverlayRendersFromKeymap(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02?"))
	s := plain(first(a))
	for _, b := range Keys("ctrl+b") {
		if !strings.Contains(s, b.Help) {
			t.Errorf("help overlay missing binding %q", b.Help)
		}
	}
	for _, want := range []string{"❓ keys", ":config", "top      cat", "any key to close"} {
		if !strings.Contains(s, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	a.HandleBytes([]byte("x")) // any key closes, and is not sent to the program
	s = plain(first(a))
	if strings.Contains(s, "❓ keys") {
		t.Fatal("help did not close")
	}
}

func TestCommandLine(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02:focus mid\r"))
	if bar := strings.Split(plain(first(a)), "\n")[0]; !strings.Contains(bar, "● mid") {
		t.Fatalf(":focus mid did not focus: %q", bar)
	}
	a.HandleBytes([]byte("\x02:bogus\r"))
	if s := plain(first(a)); !strings.Contains(s, "unknown command: bogus") {
		t.Fatalf("unknown command not reported:\n%s", strings.Split(s, "\n")[0])
	}
	// typing inside the prompt never fires mono keys
	a.HandleBytes([]byte("\x02:qjk"))
	if a.Quit() {
		t.Fatal("typing q inside the prompt quit mono")
	}
	a.HandleBytes([]byte("\x1b")) // esc cancels the prompt
	a.HandleBytes([]byte("\x02:q\r"))
	if !a.Quit() {
		t.Fatal(":q did not quit")
	}
}

func TestPrefixQuit(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("q"))
	if a.Quit() {
		t.Fatal("a bare q (for the program) quit mono")
	}
	a.HandleBytes([]byte("\x02q"))
	if !a.Quit() {
		t.Fatal("ctrl+b q did not quit")
	}
}

func TestUnknownPrefixKeyIsExplained(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02z"))
	if bar := strings.Split(plain(first(a)), "\n")[0]; !strings.Contains(bar, "ctrl+b z is not a mono key") {
		t.Fatalf("bar = %q", bar)
	}
}

func TestMissingToolDropsPaneAndOthersGrow(t *testing.T) {
	cfg := testConfig()
	cfg.Panes[1].Cmd = "nvtop"
	cfg.Panes[1].Probe = ""
	a := start(t, cfg, env{missing: map[string]bool{"nvtop": true}})
	if got := strings.Join(a.PaneNames(), ","); got != "top,bot" {
		t.Fatalf("running panes = %s, want top,bot", got)
	}
	if c := a.Choices()[1]; c.Usable() || !strings.Contains(c.Reason, "nvtop not installed") {
		t.Fatalf("choice = %+v", c)
	}
	// title 2, top 35, bot 3 at 40 rows: the every pane starts on row 37.
	s := eventually(t, a, "status line", func(s string) bool { return strings.Contains(s, "status-line") })
	if row := strings.Count(s[:strings.Index(s, "status-line")], "\n"); row != 37 {
		t.Fatalf("status line on row %d, want 37:\n%s", row, s)
	}
}

func TestExitedPaneShowsRestartHint(t *testing.T) {
	cfg := testConfig()
	cfg.Panes[0].Cmd = "sh -c 'exit 4'"
	a := start(t, cfg, env{})
	eventually(t, a, "exit notice", func(s string) bool {
		return strings.Contains(s, "top exited (status 4) — ctrl+b r restart")
	})
}

func TestResizeRelayouts(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.Resize(30, 24)
	rows, _ := a.Frame()
	if len(rows) != 24 || width(rows[5]) != 30 {
		t.Fatalf("after resize: %d rows, row width %d", len(rows), width(rows[5]))
	}
}

func TestReloadAppliesConfigLive(t *testing.T) {
	cfg := testConfig()
	a, err := New(Options{Config: cfg, Env: env{}, Title: "t", ConfigPath: "x",
		Load: func(string) (config.Config, error) {
			c := testConfig()
			c.Panes = c.Panes[:2]
			c.Panes[1].Cmd = "sh -c 'echo swapped; exec cat'"
			return c, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 40)
	t.Cleanup(a.Stop)
	a.HandleBytes([]byte("keepme\r"))
	eventually(t, a, "typed text", func(s string) bool { return strings.Contains(s, "keepme") })
	a.HandleBytes([]byte("\x02:reload\r"))
	s := eventually(t, a, "reloaded layout", func(s string) bool { return strings.Contains(s, "swapped") })
	if !strings.Contains(s, "keepme") {
		t.Fatal("unchanged pane was restarted by reload (its screen was lost)")
	}
	if got := strings.Join(a.PaneNames(), ","); got != "top,mid" {
		t.Fatalf("panes after reload = %s", got)
	}
}

func TestConfigModalRunsAndReloads(t *testing.T) {
	cfg := testConfig()
	reloads := 0
	a, err := New(Options{Config: cfg, Env: env{}, Title: "t", ConfigPath: "x",
		ConfigArgv: func() []string { return []string{"/bin/sh", "-c", "echo editing; read x"} },
		Load:       func(string) (config.Config, error) { reloads++; return testConfig(), nil }})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 20)
	t.Cleanup(a.Stop)
	a.HandleBytes([]byte("\x02:config\r"))
	eventually(t, a, "modal editor full screen", func(s string) bool { return strings.HasPrefix(s, "editing") })
	a.HandleBytes([]byte("\r")) // keys go to the editor; it exits
	eventually(t, a, "back to the dashboard", func(s string) bool { return strings.Contains(s, "● top") })
	if reloads == 0 {
		t.Fatal("config was not reloaded after the editor closed")
	}
}

func TestBadPrefixRejected(t *testing.T) {
	cfg := testConfig()
	cfg.Prefix = "ctrl+c"
	if _, err := New(Options{Config: cfg}); err == nil {
		t.Fatal("ctrl+c prefix accepted")
	}
}

// TestReloadWhileTypingAndResizing: reload runs in the background while keys
// arrive and the window changes size (the review's crash: a pane resized
// before it started; and a data race on the prefix). Run with -race.
func TestReloadWhileTypingAndResizing(t *testing.T) {
	n := 0
	a, err := New(Options{Config: testConfig(), Env: env{}, Title: "t", ConfigPath: "x",
		Load: func(string) (config.Config, error) {
			n++
			c := testConfig()
			c.Panes[1].Cmd = "sh -c 'echo v" + string(rune('0'+n%10)) + "; exec cat'"
			c.Prefix = []string{"ctrl+b", "ctrl+g"}[n%2]
			return c, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 30)
	t.Cleanup(a.Stop)
	for i := 0; i < 6; i++ {
		a.HandleBytes([]byte("\x02:reload\r"))
		a.HandleBytes([]byte("\x07:reload\r")) // whichever prefix is live
		a.Resize(50+i, 25+i)
		a.HandleBytes([]byte("typing"))
		_, _ = a.Frame()
	}
	eventually(t, a, "settled layout", func(s string) bool { return strings.Contains(s, "● ") })
}

func TestNewestReloadWins(t *testing.T) {
	calls := 0
	a, err := New(Options{Config: testConfig(), Env: env{}, Title: "t", ConfigPath: "x",
		Load: func(string) (config.Config, error) {
			calls++
			c := testConfig()
			if calls == 3 {
				c.Panes = c.Panes[:1] // the newest request: top only
			}
			return c, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	a.Resize(60, 30)
	t.Cleanup(a.Stop)
	for i := 0; i < 3; i++ {
		a.HandleBytes([]byte("\x02:reload\r"))
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && strings.Join(a.PaneNames(), ",") != "top" {
		time.Sleep(20 * time.Millisecond)
	}
	if got := strings.Join(a.PaneNames(), ","); got != "top" {
		t.Fatalf("panes = %s, want the newest config (top)", got)
	}
}

func TestHelpOnATinyTerminalDoesNotPanic(t *testing.T) {
	a := start(t, testConfig(), env{})
	a.HandleBytes([]byte("\x02?"))
	for _, w := range []int{1, 2, 3, 4, 5, 10} {
		a.Resize(w, 20)
		rows, _ := a.Frame()
		if len(rows) != 20 {
			t.Fatalf("width %d: %d rows", w, len(rows))
		}
	}
	a.Resize(60, 2)
	_, _ = a.Frame()
}
