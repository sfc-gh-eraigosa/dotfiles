// Package sshcfg inspects the ssh configuration for settings that turn a
// transient network blip into an indefinite hang.
//
// The specific failure this exists for: over a tunnel, an SSH connection can
// stall with TCP ESTABLISHED and zero bytes moving in either direction. With
// keepalives disabled — which is the DEFAULT — neither end ever notices, so
// `git fetch` hangs forever rather than failing and being retried. One
// observed instance hung 45 seconds before being killed, while the next three
// attempts over the same tunnel completed in 2–3 seconds.
package sshcfg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Cmd runs an external command; injectable so tests need no ssh binary.
type Cmd interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecCmd is the real implementation.
type ExecCmd struct{}

func (ExecCmd) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Settings are the effective values for one host.
type Settings struct {
	ServerAliveInterval int
	ServerAliveCountMax int
	ConnectTimeout      string
	Hostname            string
	Port                string
}

// KeepaliveDisabled reports the condition that makes a stall unbounded.
//
// Interval 0 is ssh's default and means "never send keepalives", so a
// connection that stops moving is never detected as dead.
func (s Settings) KeepaliveDisabled() bool { return s.ServerAliveInterval <= 0 }

// Effective asks ssh itself what applies to a host.
//
// `ssh -G` is authoritative. Re-implementing ssh's first-match-wins Host
// resolution by hand is where a checker would quietly disagree with the client
// it is trying to describe — and a wrong "all clear" is worse than no check.
func Effective(ctx context.Context, cmd Cmd, sshBin, host string) (Settings, error) {
	if sshBin == "" {
		sshBin = "ssh"
	}
	out, err := cmd.Run(ctx, sshBin, "-G", host)
	if err != nil {
		return Settings{}, fmt.Errorf("asking ssh for the effective config of %s: %w", host, err)
	}
	s := Settings{}
	for line := range strings.SplitSeq(string(out), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		switch strings.ToLower(key) {
		case "serveraliveinterval":
			s.ServerAliveInterval, _ = strconv.Atoi(value)
		case "serveralivecountmax":
			s.ServerAliveCountMax, _ = strconv.Atoi(value)
		case "connecttimeout":
			s.ConnectTimeout = value
		case "hostname":
			s.Hostname = value
		case "port":
			s.Port = value
		}
	}
	return s, nil
}

// ManagedMarker identifies the block wlink appends, so a re-run recognises its
// own work instead of adding a second copy.
const ManagedMarker = "# wlink: keepalive"

// KeepaliveInterval and KeepaliveCountMax bound a stall at roughly a minute:
// long enough not to disturb a healthy idle connection, short enough that a
// hang becomes a retryable error rather than something to notice by hand.
const (
	KeepaliveInterval = 20
	KeepaliveCountMax = 3
)

// ApplyKeepalive appends a marked block enabling keepalives for one host.
//
// Appends rather than rewrites: the rest of the file is someone else's
// configuration. Note that ssh takes the FIRST value it sees for each keyword,
// so an existing earlier block that already sets ServerAliveInterval will still
// win — which is why the caller re-checks with `ssh -G` afterwards and reports
// what actually took effect rather than assuming.
func ApplyKeepalive(path, host string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(existing), ManagedMarker) {
		return false, nil // already ours; nothing to do
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}

	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteString("\n")
	}
	if len(existing) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s — without this, a stalled connection hangs forever instead of failing.\n", ManagedMarker)
	fmt.Fprintf(&b, "Host %s\n", host)
	fmt.Fprintf(&b, "    ServerAliveInterval %d\n", KeepaliveInterval)
	fmt.Fprintf(&b, "    ServerAliveCountMax %d\n", KeepaliveCountMax)

	// 0600: ssh is strict about config permissions, and a file wlink creates
	// should never become the reason ssh starts complaining.
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return false, err
	}
	return true, nil
}
