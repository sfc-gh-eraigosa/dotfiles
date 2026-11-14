// Package agent checks and loads keys in ssh-agent. A passphrase reaches
// ssh-add only through fleet acting as its own SSH_ASKPASS program, over a
// one-shot unix socket in a 0700 temp dir; never argv, env, or a file.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// Exec runs name with args. env == nil inherits os.Environ(); otherwise the
// child sees exactly env. It returns stdout; errors carry exit status and stderr.
type Exec func(ctx context.Context, name string, env []string, args ...string) ([]byte, error)

// ErrNoAgent means SSH_AUTH_SOCK is unset or the agent cannot be reached.
var ErrNoAgent = errors.New("agent: no ssh-agent reachable (SSH_AUTH_SOCK unset or dead)")

// exitCoder matches *exec.ExitError and test fakes alike.
type exitCoder interface{ ExitCode() int }

// SystemExec is an Exec over os/exec.
func SystemExec(ctx context.Context, name string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env // nil inherits os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Loaded reports whether the key with fingerprint fp ("SHA256:…") is in the
// agent at SSH_AUTH_SOCK.
func Loaded(ctx context.Context, x Exec, fp string) (bool, error) {
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		return false, ErrNoAgent
	}
	out, err := x(ctx, "ssh-add", nil, "-l")
	if err != nil {
		var ec exitCoder
		if errors.As(err, &ec) && ec.ExitCode() == 1 { // "The agent has no identities."
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", ErrNoAgent, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) >= 2 && f[1] == fp {
			return true, nil
		}
	}
	return false, nil
}

// Load adds keyPath to the agent. It serves passphrase once over
// <0700 tmpdir>/askpass.sock and runs `ssh-add [--apple-use-keychain] keyPath`
// with SSH_ASKPASS=self, SSH_ASKPASS_REQUIRE=force (OpenSSH >= 8.4) and
// FLEET_ASKPASS_SOCK=<sock>. self must be the fleet binary (or anything that
// calls AskpassClient when FLEET_ASKPASS_SOCK is set). Success means ssh-add
// exited 0, whether or not it asked (Apple's ssh-add may take the passphrase
// from the keychain); verify with Loaded. Load blocks while ssh-add runs, so
// pass a ctx with a deadline.
func Load(ctx context.Context, x Exec, self, keyPath string, passphrase vault.Secret, goos string) error {
	if len(passphrase) == 0 {
		return errors.New("agent: empty passphrase")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv, err := startAskpass(ctx, passphrase)
	if err != nil {
		return err
	}
	defer srv.Close()

	env := make([]string, 0, len(os.Environ())+3)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "SSH_ASKPASS") || strings.HasPrefix(e, "FLEET_ASKPASS_SOCK=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "SSH_ASKPASS="+self, "SSH_ASKPASS_REQUIRE=force", "FLEET_ASKPASS_SOCK="+srv.Sock)

	name, args := sshAdd(goos, keyPath)
	if _, err := x(ctx, name, env, args...); err != nil {
		return fmt.Errorf("agent: ssh-add %s: %w", keyPath, err)
	}
	return nil
}

// appleSSHAdd is Apple's ssh-add, the only one that knows
// --apple-use-keychain; Homebrew's upstream openssh, often first in PATH,
// rejects the flag.
const appleSSHAdd = "/usr/bin/ssh-add"

// sshAdd picks the ssh-add binary and args for goos.
func sshAdd(goos, keyPath string) (string, []string) {
	if goos == "darwin" {
		return appleSSHAdd, []string{"--apple-use-keychain", keyPath}
	}
	return "ssh-add", []string{keyPath}
}
