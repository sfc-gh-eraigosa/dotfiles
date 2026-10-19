package pane

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hinshun/vt10x"
)

// Every child here is /bin/sh or a coreutil, never this test binary.

func text(v View) string {
	var b strings.Builder
	for _, r := range v.Rows {
		for i := 0; i < len(r); i++ {
			if r[i] == 0x1b {
				for i < len(r) && r[i] != 'm' {
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

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestInteractiveKeysReachProgram(t *testing.T) {
	p := New(Spec{Name: "cat", Cmd: "cat"}, nil)
	if err := p.Start(40, 5); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	p.Write([]byte("hello mono\r"))
	waitFor(t, "echoed text", func() bool { return strings.Contains(text(p.Snapshot(40, 5)), "hello mono") })
	v := p.Snapshot(40, 5)
	if len(v.Rows) != 5 {
		t.Fatalf("snapshot has %d rows, want 5", len(v.Rows))
	}
}

func TestInteractiveExitIsReported(t *testing.T) {
	p := New(Spec{Name: "x", Cmd: "sh -c 'echo bye; exit 3'"}, nil)
	if err := p.Start(20, 3); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	waitFor(t, "exit", func() bool { e, _ := p.Exited(); return e })
	if _, why := p.Exited(); why != "exited (status 3)" {
		t.Fatalf("exit reason = %q", why)
	}
	if !strings.Contains(text(p.Snapshot(20, 3)), "bye") {
		t.Fatal("output before exit not kept on screen")
	}
}

func TestRestartRunsAgain(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "n")
	p := New(Spec{Name: "x", Cmd: "sh -c 'echo run >> " + f + "; exec cat'"}, nil)
	if err := p.Start(20, 3); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	waitFor(t, "first run", func() bool { b, _ := os.ReadFile(f); return strings.Count(string(b), "run") == 1 })
	if err := p.Restart(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second run", func() bool { b, _ := os.ReadFile(f); return strings.Count(string(b), "run") == 2 })
}

func TestEveryReRunsAndKeepsLastLine(t *testing.T) {
	dir := t.TempDir()
	n := filepath.Join(dir, "n")
	// Prints an increasing counter followed by a newline, like gsl status.
	cmd := "sh -c 'c=$(cat " + n + " 2>/dev/null || echo 0); c=$((c+1)); echo $c > " + n + "; echo tick-$c'"
	p := New(Spec{Name: "e", Cmd: cmd, Every: 100 * time.Millisecond}, nil)
	if err := p.Start(20, 1); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	waitFor(t, "third run shown in a one-row pane", func() bool {
		s := text(p.Snapshot(20, 1))
		i := strings.Index(s, "tick-")
		if i < 0 {
			return false
		}
		k, _ := strconv.Atoi(strings.TrimSpace(s[i+5:]))
		return k >= 3
	})
	if p.Interactive() {
		t.Fatal("an every pane must not take keystrokes")
	}
}

func TestResizeReachesProgram(t *testing.T) {
	p := New(Spec{Name: "s", Cmd: "stty size", Every: 100 * time.Millisecond}, nil)
	if err := p.Start(30, 2); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	waitFor(t, "initial size", func() bool { return strings.Contains(text(p.Snapshot(30, 2)), "2 30") })
	p.Resize(50, 3)
	waitFor(t, "new size", func() bool { return strings.Contains(text(p.Snapshot(50, 3)), "3 50") })

	q := New(Spec{Name: "i", Cmd: "sh -c 'trap \"stty size\" WINCH; while :; do sleep 0.05; done'"}, nil)
	if err := q.Start(30, 4); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	time.Sleep(200 * time.Millisecond)
	q.Resize(44, 6)
	waitFor(t, "interactive SIGWINCH", func() bool { return strings.Contains(text(q.Snapshot(44, 6)), "6 44") })
}

func TestStopKillsTheWholeProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidf := filepath.Join(dir, "pid")
	// The shell ignores SIGHUP and leaves a grandchild behind: Stop must
	// still clear both (SIGKILL to the group after the grace period).
	p := New(Spec{Name: "g", Cmd: "sh -c 'trap \"\" HUP; sleep 300 & echo $! > " + pidf + "; wait'"}, nil)
	if err := p.Start(20, 3); err != nil {
		t.Fatal(err)
	}
	var grand int
	waitFor(t, "grandchild pid", func() bool {
		b, err := os.ReadFile(pidf)
		grand, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil && grand > 0
	})
	p.Stop()
	waitFor(t, "grandchild gone", func() bool { return syscall.Kill(grand, 0) != nil })
	p.Stop() // idempotent
}

func TestStartFailureIsShown(t *testing.T) {
	p := New(Spec{Name: "x", Cmd: "x", Dir: "/nonexistent-dir-for-mono-test"}, nil)
	_ = p.Start(20, 2)
	defer p.Stop()
	if e, why := p.Exited(); !e || !strings.Contains(why, "could not start") {
		t.Fatalf("Exited() = %v %q", e, why)
	}
}

// TestEveryAnswersTerminalQueries: programs that ask the terminal something
// on startup (bubbletea's init asks for the background color, then the cursor
// position, and waits up to 5s) must get an answer from an every pane, as
// they do from a real terminal. gsl status stalled 5s per run before this.
func TestEveryAnswersTerminalQueries(t *testing.T) {
	cmd := `sh -c 'stty raw -echo; printf "\033[6n"; head -c 6 >/dev/null; stty sane; echo answered'`
	p := New(Spec{Name: "q", Cmd: cmd, Every: time.Second}, nil)
	start := time.Now()
	if err := p.Start(40, 2); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	waitFor(t, "the query answered", func() bool { return strings.Contains(text(p.Snapshot(40, 2)), "answered") })
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("query took %s to be answered", d)
	}
}

func readPids(t *testing.T, f string) []int { //nolint:unparam
	t.Helper()
	b, _ := os.ReadFile(f)
	var out []int
	for _, l := range strings.Fields(string(b)) {
		if n, err := strconv.Atoi(l); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestResizeAndSnapshotBeforeStart(t *testing.T) {
	p := New(Spec{Name: "x", Cmd: "cat"}, nil)
	p.Resize(30, 4) // used to panic: no emulator until Start
	if v := p.Snapshot(30, 4); len(v.Rows) != 4 {
		t.Fatalf("snapshot rows = %d", len(v.Rows))
	}
	p.Stop()
	if err := p.Start(30, 4); err != nil { // after Stop: a no-op, nothing runs
		t.Fatal(err)
	}
	if e, _ := p.Exited(); e {
		t.Fatal("Start after Stop ran something")
	}
}

// sleepers lists live processes running `sleep <tag>`.
func sleepers(tag string) []int {
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	var out []int
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "cmdline"))
		if err == nil && string(b) == "sleep\x00"+tag+"\x00" {
			n, _ := strconv.Atoi(filepath.Base(d))
			out = append(out, n)
		}
	}
	return out
}

func TestConcurrentRestartsThenStopLeaveNothing(t *testing.T) {
	// sleep inherits the ignored SIGHUP, so only the group SIGKILL ends it.
	p := New(Spec{Name: "r", Cmd: "sh -c 'trap \"\" HUP; exec sleep 7731'"}, nil)
	if err := p.Start(20, 2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first program", func() bool { return len(sleepers("7731")) == 1 })
	done := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() { _ = p.Restart(); done <- struct{}{} }()
	}
	<-done
	<-done
	waitFor(t, "exactly one program after two restarts", func() bool { return len(sleepers("7731")) == 1 })
	p.Stop()
	_ = p.Restart() // after Stop: must not start anything
	waitFor(t, "nothing left", func() bool { return len(sleepers("7731")) == 0 })
	time.Sleep(200 * time.Millisecond)
	if n := len(sleepers("7731")); n != 0 {
		t.Fatalf("%d program(s) running after Stop + Restart", n)
	}
}

func TestStopKillsGroupMembersThatIgnoreHUP(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pid")
	// The leader obeys SIGHUP and exits at once; its child ignores it.
	p := New(Spec{Name: "g", Cmd: "sh -c '(trap \"\" HUP; exec sleep 7733) & echo $! > " + f + "; wait'"}, nil)
	if err := p.Start(20, 2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "child pid", func() bool { return len(readPids(t, f)) == 1 })
	child := readPids(t, f)[0]
	p.Stop()
	waitFor(t, "child gone", func() bool { return !alive(child) })
}

func TestStopKillsCaptureInFlight(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pid")
	p := New(Spec{Name: "c", Cmd: "sh -c 'echo $$ > " + f + "; exec sleep 7732'", Every: 100 * time.Millisecond}, nil)
	if err := p.Start(20, 1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "capture started", func() bool { return len(readPids(t, f)) == 1 })
	pid := readPids(t, f)[0]
	start := time.Now()
	p.Stop()
	waitFor(t, "capture gone", func() bool { return !alive(pid) })
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("capture outlived Stop by %s (it waited for its own timeout)", d)
	}
}

func TestWriteNeverBlocks(t *testing.T) {
	p := New(Spec{Name: "s", Cmd: "sleep 30"}, nil) // never reads its input
	if err := p.Start(20, 2); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	chunk := []byte(strings.Repeat("x", 4096))
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ { // 8 MB, far beyond the pty buffer
			p.Write(chunk)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Write blocked on a program that is not reading")
	}
}

func TestSplitUTF8(t *testing.T) {
	braille := "⣿" // 3 bytes
	b := []byte("ab" + braille)
	for cut := 3; cut < len(b); cut++ {
		complete, tail := SplitUTF8(b[:cut])
		if string(complete) != "ab" || len(tail) != cut-2 {
			t.Errorf("cut %d: complete %q tail %q", cut, complete, tail)
		}
	}
	if c, tail := SplitUTF8(b); string(c) != string(b) || tail != nil {
		t.Errorf("whole: %q %q", c, tail)
	}
	if c, tail := SplitUTF8([]byte("plain")); string(c) != "plain" || tail != nil {
		t.Errorf("ascii: %q %q", c, tail)
	}
}

// chunks returns its pieces one Read at a time.
type chunks struct{ parts [][]byte }

func (c *chunks) Read(b []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.parts[0])
	c.parts = c.parts[1:]
	return n, nil
}

func TestPumpKeepsRunesSplitAcrossReads(t *testing.T) {
	s := []byte("⣿⣿⣿")
	vt := vt10x.New(vt10x.WithSize(10, 1))
	p := New(Spec{Name: "u"}, nil)
	p.pump(&chunks{parts: [][]byte{s[:1], s[1:4], s[4:8], s[8:]}}, vt, false)
	vt.Lock()
	got := string([]rune{vt.Cell(0, 0).Char, vt.Cell(1, 0).Char, vt.Cell(2, 0).Char})
	vt.Unlock()
	if got != "⣿⣿⣿" {
		t.Fatalf("screen = %q, want ⣿⣿⣿", got)
	}
}
