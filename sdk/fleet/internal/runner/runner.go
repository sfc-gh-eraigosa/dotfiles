// Package runner is the single seam through which fleet touches a remote
// machine. Everything else in the tool is pure, so tests never open a socket.
package runner

import (
	"errors"
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

func (e Exec) RunStdin(host, stdin string, argv ...string) (string, error) {
	base := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.timeout(), host}
	c := exec.Command("ssh", append(base, argv...)...)
	c.Stdin = strings.NewReader(stdin)
	// CombinedOutput: sudo writes its failure text to stderr, and that text is
	// the whole diagnosis when authentication fails.
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ErrFake is the canned failure used by Fake.
var ErrFake = errors.New("runner: fake ssh failure")

// Fake is a table-driven Runner for tests. Stdin records what was piped to
// each host so a test can assert a secret went over stdin and not argv.
type Fake struct {
	Out   map[string]string
	Err   map[string]error
	Stdin map[string]string
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

func (f Fake) RunStdin(host, stdin string, _ ...string) (string, error) {
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}
