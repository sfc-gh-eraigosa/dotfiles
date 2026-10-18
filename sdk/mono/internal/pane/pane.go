// Package pane runs one program in a pseudo-terminal and keeps an emulated
// screen of what it drew. A pane is either long-lived and interactive (htop),
// or re-run on an interval with its latest output shown (gsl status).
package pane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/screen"
)

// KillGrace is how long a program gets after SIGHUP before SIGKILL.
const KillGrace = 500 * time.Millisecond

// inputQueue bounds keystrokes (and terminal replies) waiting for a program
// that is not reading. Beyond it input is dropped rather than stalling mono.
const inputQueue = 256

// Spec describes what a pane runs.
type Spec struct {
	Name  string
	Cmd   string        // run through /bin/sh -c "exec <Cmd>"
	Every time.Duration // > 0: re-run on this interval instead of staying up
	Dir   string        // working directory; "" = mono's
}

// Pane is one running program and its screen. All methods are safe for
// concurrent use; Notify is called (from any goroutine, never with a pane
// lock held) whenever the screen may have changed.
type Pane struct {
	Spec   Spec
	Notify func()

	life sync.Mutex // serializes Start, Restart and Stop

	mu      sync.Mutex
	vt      vt10x.Terminal // never nil
	cols    int
	rows    int
	tty     *os.File
	input   *queue // the running program's input; nil when none
	proc    *os.Process
	capProc *os.Process // an Every capture in flight
	exited  bool
	exitMsg string
	stop    chan struct{} // closed by Stop
	stopped bool
	busy    bool // an Every capture is in flight
}

// New builds a pane; nothing runs until Start. Resize and Snapshot are safe
// before Start.
func New(s Spec, notify func()) *Pane {
	if notify == nil {
		notify = func() {}
	}
	return &Pane{Spec: s, Notify: notify, stop: make(chan struct{}), vt: vt10x.New(vt10x.WithSize(1, 1)), cols: 1, rows: 1}
}

// Start launches the program at the given size. It does nothing after Stop.
func (p *Pane) Start(cols, rows int) error {
	p.life.Lock()
	defer p.life.Unlock()
	cols, rows = max(cols, 1), max(rows, 1)
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.cols, p.rows = cols, rows
	vrows := rows
	if p.Spec.Every > 0 {
		vrows++
	}
	p.vt = vt10x.New(vt10x.WithSize(cols, vrows))
	p.mu.Unlock()
	if p.Spec.Every > 0 {
		go p.everyLoop()
		return nil
	}
	return p.startInteractive()
}

func (p *Pane) command() *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", "exec "+p.Spec.Cmd)
	cmd.Dir = p.Spec.Dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MONO_PANE="+p.Spec.Name)
	return cmd
}

// startInteractive must be called with p.life held.
func (p *Pane) startInteractive() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	cols, rows := p.cols, p.rows
	p.mu.Unlock()
	cmd := p.command()
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		p.setExited(fmt.Sprintf("could not start %q: %v", p.Spec.Cmd, err))
		return err
	}
	in := newQueue(tty)
	// The emulator answers the program's terminal queries (cursor position,
	// device attributes, colors) like a real terminal, through the same
	// non-blocking queue as keystrokes: vt10x replies while holding its own
	// lock, so a direct write to a full pty would freeze every snapshot.
	vt := vt10x.New(vt10x.WithSize(cols, rows), vt10x.WithWriter(in))
	p.mu.Lock()
	p.vt, p.tty, p.input, p.proc, p.exited, p.exitMsg = vt, tty, in, cmd.Process, false, ""
	p.mu.Unlock()
	go p.pump(tty, vt, true)
	go func() {
		err := cmd.Wait()
		// Detach the tty under the lock before closing it: Resize uses its
		// raw descriptor (ioctl) while holding the lock.
		p.mu.Lock()
		current := p.proc == cmd.Process
		if current {
			p.proc, p.tty, p.input = nil, nil, nil
		}
		p.mu.Unlock()
		in.close()
		_ = tty.Close()
		if current {
			p.setExited(exitText(err))
		}
	}()
	return nil
}

// pump feeds program output into the emulator. vt10x drops a UTF-8 rune
// split across two writes, so an incomplete trailing rune is carried over to
// the next read (htop and nvtop graphs are multi-byte braille/blocks).
func (p *Pane) pump(r io.Reader, vt vt10x.Terminal, notify bool) {
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			complete, tail := SplitUTF8(pending)
			if len(complete) > 0 {
				_, _ = vt.Write(complete)
			}
			pending = append(pending[:0], tail...)
			if notify {
				p.Notify()
			}
		}
		if err != nil {
			return
		}
	}
}

// SplitUTF8 splits b before a trailing incomplete UTF-8 sequence.
func SplitUTF8(b []byte) (complete, tail []byte) {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if b[i]&0xC0 != 0x80 { // a start byte (or ASCII)
			if !utf8.FullRune(b[i:]) {
				return b[:i], b[i:]
			}
			break
		}
	}
	return b, nil
}

func (p *Pane) setExited(msg string) {
	p.mu.Lock()
	p.exited, p.exitMsg = true, msg
	p.mu.Unlock()
	p.Notify()
}

func exitText(err error) string {
	var ee *exec.ExitError
	switch {
	case err == nil:
		return "exited"
	case errors.As(err, &ee) && ee.ExitCode() >= 0:
		return fmt.Sprintf("exited (status %d)", ee.ExitCode())
	default:
		return "exited (" + err.Error() + ")"
	}
}

func (p *Pane) everyLoop() {
	t := time.NewTicker(p.Spec.Every)
	defer t.Stop()
	p.capture()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.capture()
		}
	}
}

// capture runs the command once in a fresh pty and swaps its screen in when
// it finishes. A run still going when the next tick arrives is not doubled;
// one running longer than max(5s, 5 ticks) is killed.
func (p *Pane) capture() {
	p.mu.Lock()
	if p.busy || p.stopped {
		p.mu.Unlock()
		return
	}
	p.busy = true
	cols, rows := p.cols, p.rows
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.busy, p.capProc = false, nil
		p.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), max(5*time.Second, 5*p.Spec.Every))
	defer cancel()
	cmd := p.command()
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		p.setExited(fmt.Sprintf("could not start %q: %v", p.Spec.Cmd, err))
		return
	}
	p.mu.Lock()
	stopped := p.stopped
	p.capProc = cmd.Process
	p.mu.Unlock()
	if stopped { // Stop ran while we were starting
		killGroup(cmd.Process, syscall.SIGKILL)
	}
	in := newQueue(tty)
	// One spare row: a command that ends with a newline (gsl does) would
	// otherwise scroll its only line out of a one-row pane. The writer lets
	// the emulator answer terminal queries (cursor position, background
	// color): bubbletea asks both at startup and waits up to 5s for a reply,
	// which made every `gsl status` run time out.
	vt := vt10x.New(vt10x.WithSize(cols, rows+1), vt10x.WithWriter(in))
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			killGroup(cmd.Process, syscall.SIGKILL)
		}
	}()
	pumped := make(chan struct{})
	go func() {
		defer close(pumped)
		p.pump(tty, vt, false)
	}()
	werr := cmd.Wait()
	in.close()
	_ = tty.Close()
	<-pumped
	p.mu.Lock()
	if !p.stopped {
		p.vt = vt
		p.exited, p.exitMsg = false, ""
		if werr != nil && ctx.Err() != nil {
			p.exited, p.exitMsg = true, "timed out"
		}
	}
	p.mu.Unlock()
	p.Notify()
}

// Resize changes the pane's size; an interactive program gets SIGWINCH.
func (p *Pane) Resize(cols, rows int) {
	cols, rows = max(cols, 1), max(rows, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	if cols == p.cols && rows == p.rows {
		return
	}
	p.cols, p.rows = cols, rows
	if p.Spec.Every > 0 {
		p.vt.Resize(cols, rows+1)
		return
	}
	p.vt.Resize(cols, rows)
	if p.tty != nil {
		_ = pty.Setsize(p.tty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
}

// Write queues keystrokes for an interactive program. It never blocks: input
// beyond the queue (a huge paste into a program that is not reading) is
// dropped.
func (p *Pane) Write(b []byte) {
	p.mu.Lock()
	in := p.input
	p.mu.Unlock()
	if in != nil {
		_, _ = in.Write(b)
	}
}

// Interactive reports whether the pane takes keystrokes.
func (p *Pane) Interactive() bool { return p.Spec.Every == 0 }

// Restart stops the program and starts it again at the current size. It does
// nothing after Stop.
func (p *Pane) Restart() error {
	if p.Spec.Every > 0 {
		go p.capture()
		return nil
	}
	p.life.Lock()
	defer p.life.Unlock()
	p.mu.Lock()
	proc, stopped := p.proc, p.stopped
	p.mu.Unlock()
	if stopped {
		return nil
	}
	if proc != nil {
		terminate(proc)
	}
	return p.startInteractive()
}

// Stop ends the program (SIGHUP to its process group, then SIGKILL to the
// group after KillGrace), any Every capture in flight, and the refresh loop.
// It is safe to call more than once; nothing starts after it.
func (p *Pane) Stop() {
	p.life.Lock()
	defer p.life.Unlock()
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	close(p.stop)
	proc, capProc := p.proc, p.capProc
	p.mu.Unlock()
	if capProc != nil {
		killGroup(capProc, syscall.SIGKILL)
	}
	if proc != nil {
		terminate(proc)
	}
}

// Exited reports whether an interactive program has ended, and how.
func (p *Pane) Exited() (bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited, p.exitMsg
}

// View is a snapshot of the pane for drawing.
type View struct {
	Rows       []string // exactly the pane's height, each exactly its width
	Cursor     screen.Cursor
	Exited     bool
	ExitReason string
}

// Snapshot renders the pane at width x height.
func (p *Pane) Snapshot(width, height int) View {
	p.mu.Lock()
	vt := p.vt
	v := View{Exited: p.exited, ExitReason: p.exitMsg}
	p.mu.Unlock()
	v.Rows = make([]string, height)
	vt.Lock()
	for y := 0; y < height; y++ {
		v.Rows[y] = screen.Row(vt, y, width)
	}
	c := vt.Cursor()
	v.Cursor = screen.Cursor{X: c.X, Y: c.Y, Show: vt.CursorVisible() && c.Y < height && c.X < width}
	vt.Unlock()
	return v
}

// terminate sends SIGHUP to the process group, waits up to KillGrace for the
// leader to go, then SIGKILLs the whole group regardless: members that
// ignore SIGHUP must not outlive a leader that obeyed it.
func terminate(proc *os.Process) {
	killGroup(proc, syscall.SIGHUP)
	for i := 0; i < int(KillGrace/(20*time.Millisecond)); i++ {
		if syscall.Kill(proc.Pid, 0) != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	killGroup(proc, syscall.SIGKILL)
}

// killGroup signals the program's whole process group: pty.Start makes the
// program a session leader, so its pid is the group id.
func killGroup(proc *os.Process, sig syscall.Signal) {
	if proc == nil {
		return
	}
	if err := syscall.Kill(-proc.Pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = proc.Signal(sig)
	}
}

// queue is a bounded, non-blocking writer in front of a pty: Write copies and
// enqueues (dropping when full); one goroutine drains it into the pty.
type queue struct {
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

func newQueue(w io.Writer) *queue {
	q := &queue{ch: make(chan []byte, inputQueue), done: make(chan struct{})}
	go func() {
		for {
			select {
			case b := <-q.ch:
				if _, err := w.Write(b); err != nil {
					return
				}
			case <-q.done:
				return
			}
		}
	}()
	return q
}

func (q *queue) Write(b []byte) (int, error) {
	c := append([]byte(nil), b...)
	select {
	case <-q.done:
	case q.ch <- c:
	default: // full: drop rather than block the caller
	}
	return len(b), nil
}

func (q *queue) close() { q.once.Do(func() { close(q.done) }) }
