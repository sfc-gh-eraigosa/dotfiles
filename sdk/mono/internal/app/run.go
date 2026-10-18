package app

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/screen"
	"golang.org/x/term"
)

// frameEvery caps redraws (~30 fps); a frame is only drawn when something
// changed.
const frameEvery = 33 * time.Millisecond

// ErrNotTerminal is returned when stdin or stdout is not a terminal.
var ErrNotTerminal = errors.New("mono needs an interactive terminal (stdin and stdout must be a tty)")

// Run takes over the terminal (raw mode, alternate screen) until the user
// quits or the terminal goes away, then stops every pane and restores it.
func Run(o Options, in, out *os.File) (err error) {
	inFd, outFd := int(in.Fd()), int(out.Fd())
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return ErrNotTerminal
	}
	w, h, err := term.GetSize(outFd)
	if err != nil {
		return err
	}
	state, err := term.MakeRaw(inFd)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[2J")
	defer func() {
		_, _ = io.WriteString(out, "\x1b[0m\x1b[?25h\x1b[?1049l")
		_ = term.Restore(inFd, state)
	}()

	dirty := make(chan struct{}, 1)
	o.OnChange = func() {
		select {
		case dirty <- struct{}{}:
		default:
		}
	}
	a, err := New(o)
	if err != nil {
		return err
	}
	defer a.Stop()
	a.Resize(w, h)

	keys := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := in.Read(buf)
			if n > 0 {
				keys <- append([]byte(nil), buf[:n]...)
			}
			if rerr != nil {
				close(keys)
				return
			}
		}
	}()

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(sigs)

	sw := screen.NewWriter(out)
	tick := time.NewTicker(frameEvery)
	defer tick.Stop()
	draw := true
	for {
		select {
		case b, ok := <-keys:
			if !ok {
				return nil // terminal closed
			}
			a.HandleBytes(b)
			if a.Quit() {
				return nil
			}
		case s := <-sigs:
			if s != syscall.SIGWINCH {
				return nil
			}
			if nw, nh, gerr := term.GetSize(outFd); gerr == nil {
				a.Resize(nw, nh)
				sw.Invalidate()
				_, _ = io.WriteString(out, "\x1b[2J")
			}
		case <-dirty:
			draw = true
		case <-tick.C:
			if draw {
				rows, cur := a.Frame()
				if werr := sw.Draw(rows, cur); werr != nil {
					return werr
				}
				draw = false
			}
		}
	}
}
