package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// maxSockPath keeps the socket path under the smallest sun_path limit we run
// on (macOS: 104 bytes including the NUL; Linux: 108).
const maxSockPath = 100

// askpassServer hands one passphrase to exactly one client over a unix socket
// in a private 0700 directory. An inherited fd cannot be used instead:
// ssh-add closes every fd above stderr at startup.
type askpassServer struct {
	Dir  string // 0700, removed by Close
	Sock string // <Dir>/askpass.sock

	ln     net.Listener
	done   chan struct{} // closed when the serving goroutine returns
	read   atomic.Bool   // the passphrase was written to a client
	closer sync.Once
}

// askpassDir makes the private socket directory. os.MkdirTemp already uses
// 0700; the Chmod enforces it against an unusual umask or platform. On macOS
// $TMPDIR (/var/folders/…) can push the socket path past the ~104-byte
// sun_path limit, so a too-long path falls back to /tmp.
func askpassDir() (string, error) {
	for _, base := range []string{"", "/tmp"} {
		dir, err := os.MkdirTemp(base, "fa")
		if err != nil {
			return "", err
		}
		if len(filepath.Join(dir, "askpass.sock")) > maxSockPath {
			_ = os.RemoveAll(dir)
			continue
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
		return dir, nil
	}
	return "", errors.New("agent: no temp dir short enough for a unix socket")
}

// startAskpass listens and serves pass once in the background. The caller
// must Close it. ctx cancellation stops the server without serving.
func startAskpass(ctx context.Context, pass []byte) (*askpassServer, error) {
	dir, err := askpassDir()
	if err != nil {
		return nil, fmt.Errorf("agent: askpass dir: %w", err)
	}
	sock := filepath.Join(dir, "askpass.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("agent: askpass listen: %w", err)
	}
	s := &askpassServer{Dir: dir, Sock: sock, ln: ln, done: make(chan struct{})}

	// The payload is a private copy, zeroed once served or abandoned.
	payload := make([]byte, len(pass)+1)
	copy(payload, pass)
	payload[len(pass)] = '\n'

	go func() { // closes the listener on cancel; exits once serving is over
		select {
		case <-ctx.Done():
			_ = s.ln.Close()
		case <-s.done:
		}
	}()
	go func() {
		defer close(s.done)
		defer clear(payload)
		conn, err := s.ln.Accept() // unblocked by ln.Close (Close or cancel)
		_ = s.ln.Close()           // one client only: later dials are refused
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := conn.Write(payload); err == nil {
			s.read.Store(true)
		}
	}()
	return s, nil
}

// wait blocks until the serving goroutine has returned.
func (s *askpassServer) wait() { <-s.done }

// served reports whether the passphrase was written to a client.
func (s *askpassServer) served() bool { return s.read.Load() }

// Close stops the server, waits for its goroutine, and removes the directory.
func (s *askpassServer) Close() error {
	var err error
	s.closer.Do(func() {
		_ = s.ln.Close()
		s.wait()
		err = os.RemoveAll(s.Dir)
	})
	return err
}

// AskpassClient is fleet's SSH_ASKPASS mode: it reads the one-shot passphrase
// from sock and copies it (already newline-terminated) to out. main.go runs
// it when FLEET_ASKPASS_SOCK is set; ssh-add's prompt (argv[1]) is ignored.
func AskpassClient(sock string, out io.Writer) error {
	conn, err := net.DialTimeout("unix", sock, 10*time.Second)
	if err != nil {
		return fmt.Errorf("agent: askpass dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := io.Copy(out, conn)
	if err != nil {
		return fmt.Errorf("agent: askpass read: %w", err)
	}
	if n == 0 {
		return errors.New("agent: askpass: no passphrase served")
	}
	return nil
}
