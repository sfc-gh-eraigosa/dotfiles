// Package runner is the single seam through which fleet touches a remote
// machine. Everything else in the tool is pure, so tests never open a socket.
package runner

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner executes a command on a remote host.
type Runner interface {
	// Run executes non-interactively and returns stdout.
	Run(host string, argv ...string) (string, error)
	// RunInteractive hands the terminal to the remote command, so prompts
	// (notably install.sh's sudo prompt) reach the operator.
	RunInteractive(host string, argv ...string) error
	// RunStdin executes non-interactively with stdin piped to the remote
	// command. It exists so a sudo password can reach `sudo -S` WITHOUT ever
	// appearing in argv: /proc/<pid>/cmdline is world-readable on both ends,
	// so a secret passed as an argument leaks to every user on the box.
	RunStdin(host, stdin string, argv ...string) (string, error)
	// RunVia executes on host by relaying through peer (ssh -J). It exists for
	// the wake ladder: when the workstation cannot resolve a sleeping host at
	// layer 2, a peer that CAN reach it becomes the transport. The peer is a
	// hop only — authentication stays end-to-end workstation -> host, so the
	// peer never needs the workstation's keys.
	RunVia(peer, host string, argv ...string) (string, error)
	// RunStream is RunStdin that reports progress as it happens: every output
	// line is delivered on `lines` (closed when the command ends) and the final
	// error on `done`. It exists because output captured only at completion
	// tells the operator nothing while a ten-minute install is in flight.
	RunStream(host, stdin string, argv ...string) (lines <-chan string, done <-chan error)
}

// Exec is the real SSH-backed runner.
type Exec struct{ ConnectTimeout string }

func (e Exec) timeout() string {
	if e.ConnectTimeout == "" {
		return "6"
	}
	return e.ConnectTimeout
}

func (e Exec) Run(host string, argv ...string) (string, error) {
	base := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.timeout(), host}
	out, err := exec.Command("ssh", append(base, argv...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func (e Exec) RunInteractive(host string, argv ...string) error {
	c := exec.Command("ssh", append([]string{"-t", host}, argv...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// viaArgs builds the relay argv. Split out from RunVia so the ordering — which
// decides WHICH machine the command lands on — is asserted by a unit test
// instead of by opening a socket.
func viaArgs(peer, host, timeout string, argv []string) []string {
	base := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=" + timeout,
		"-J", peer,
		host,
	}
	return append(base, argv...)
}

func (e Exec) RunVia(peer, host string, argv ...string) (string, error) {
	out, err := exec.Command("ssh", viaArgs(peer, host, e.timeout(), argv)...).Output()
	return strings.TrimSpace(string(out)), err
}

func (e Exec) RunStdin(host, stdin string, argv ...string) (string, error) {
	base := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.timeout(), host}
	c := exec.Command("ssh", append(base, argv...)...)
	c.Stdin = strings.NewReader(stdin)
	// CombinedOutput: sudo writes its failure text to stderr, and that text is
	// the whole diagnosis when authentication fails.
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunStream pipes the remote command's combined output back line by line.
// stdout and stderr share one pipe so the log reads in the order the remote
// produced it — install.sh writes progress to both.
func (e Exec) RunStream(host, stdin string, argv ...string) (<-chan string, <-chan error) {
	lines := make(chan string, 256)
	done := make(chan error, 1)

	base := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.timeout(), host}
	c := exec.Command("ssh", append(base, argv...)...)
	c.Stdin = strings.NewReader(stdin)
	pr, pw := io.Pipe()
	c.Stdout, c.Stderr = pw, pw

	if err := c.Start(); err != nil {
		close(lines)
		done <- err
		return lines, done
	}
	go func() {
		// Closing the writer ends the scanner below; without it the reader
		// would block forever after the process exits.
		err := c.Wait()
		_ = pw.Close()
		done <- err
	}()
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // install logs have long lines
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines, done
}

// ErrFake is the canned failure used by Fake.
var ErrFake = errors.New("runner: fake ssh failure")

// Fake is a table-driven Runner for tests. Stdin records what was piped to
// each host so a test can assert a secret went over stdin and not argv; Via
// records which peer relayed to each host so a wake test can assert the peer
// ranking chose the peer it meant to.
type Fake struct {
	Out   map[string]string
	Err   map[string]error
	Stdin map[string]string
	Via   map[string]string // host -> peer that relayed
}

func (f Fake) Run(host string, _ ...string) (string, error) {
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}

func (f Fake) RunInteractive(host string, _ ...string) error {
	if err, ok := f.Err[host]; ok {
		return err
	}
	return nil
}

func (f Fake) RunVia(peer, host string, _ ...string) (string, error) {
	if f.Via != nil {
		f.Via[host] = peer
	}
	// The relay is only a transport: the outcome belongs to the TARGET, so a
	// reachable peer must not make an unreachable host look like a success.
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}

// RunStream replays Out[host] as lines, so a test can drive the streaming
// path deterministically with no process involved.
func (f Fake) RunStream(host, stdin string, _ ...string) (<-chan string, <-chan error) {
	lines := make(chan string, 64)
	done := make(chan error, 1)
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	go func() {
		defer close(lines)
		for _, l := range strings.Split(f.Out[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- l
			}
		}
		done <- f.Err[host]
	}()
	return lines, done
}

func (f Fake) RunStdin(host, stdin string, _ ...string) (string, error) {
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}
