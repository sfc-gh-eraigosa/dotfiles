package sshcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSSH struct {
	out  string
	err  error
	args []string
}

func (f *fakeSSH) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.args = args
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.out), nil
}

// `ssh -G` is the authoritative answer to "what settings apply to this host?" —
// far better than re-implementing ssh's first-match-wins Host resolution, which
// is where a hand-rolled parser would quietly disagree with reality.
func TestEffective_ParsesSSHDashG(t *testing.T) {
	f := &fakeSSH{out: "hostname github.com\nport 22\nserveraliveinterval 0\nserveralivecountmax 3\nconnecttimeout none\n"}
	got, err := Effective(context.Background(), f, "ssh", "github.com")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if got.ServerAliveInterval != 0 {
		t.Errorf("ServerAliveInterval = %d, want 0", got.ServerAliveInterval)
	}
	if got.ConnectTimeout != "none" {
		t.Errorf("ConnectTimeout = %q, want none", got.ConnectTimeout)
	}
	if f.args[0] != "-G" {
		t.Errorf("invoked ssh %v, want -G", f.args)
	}
}

// The whole point of the check: interval 0 means keepalives are DISABLED, so a
// stalled connection hangs forever instead of failing and being retried.
func TestSettings_KeepaliveDisabled(t *testing.T) {
	if !(Settings{ServerAliveInterval: 0}).KeepaliveDisabled() {
		t.Error("interval 0 must count as disabled")
	}
	if (Settings{ServerAliveInterval: 20}).KeepaliveDisabled() {
		t.Error("interval 20 is enabled")
	}
}

func TestEffective_SurfacesSSHFailure(t *testing.T) {
	if _, err := Effective(context.Background(), &fakeSSH{err: os.ErrNotExist}, "ssh", "github.com"); err == nil {
		t.Fatal("want the ssh failure surfaced rather than a silent zero value")
	}
}

// --fix appends a clearly-marked block. It must be idempotent, because doctor
// is the kind of command people run repeatedly.
func TestApplyKeepalive_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	original := "Host lab-pi  #fleet\n    Hostname lab-pi\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := ApplyKeepalive(path, "github.com")
	if err != nil {
		t.Fatalf("ApplyKeepalive: %v", err)
	}
	if !changed {
		t.Error("first run should have changed the file")
	}
	after := readFile(t, path)
	if !strings.Contains(after, original) {
		t.Errorf("clobbered the user's existing config:\n%s", after)
	}
	if !strings.Contains(after, "ServerAliveInterval") {
		t.Errorf("keepalive not added:\n%s", after)
	}

	changed2, err := ApplyKeepalive(path, "github.com")
	if err != nil {
		t.Fatalf("second ApplyKeepalive: %v", err)
	}
	if changed2 {
		t.Error("second run reported a change; must be idempotent")
	}
	if readFile(t, path) != after {
		t.Error("second run modified the file")
	}
}

// Someone else's Host blocks are not ours to touch.
func TestApplyKeepalive_LeavesOtherBlocksAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	original := "Host lab-pi  #fleet\n    Hostname lab-pi\n    User someone\n\nHost other\n    Port 2222\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyKeepalive(path, "github.com"); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, path)
	for _, keep := range []string{"Host lab-pi", "User someone", "Host other", "Port 2222"} {
		if !strings.Contains(after, keep) {
			t.Errorf("lost %q from the config", keep)
		}
	}
}

// A config that does not exist yet is created rather than erroring — a fresh
// machine is a normal state, not a failure.
func TestApplyKeepalive_CreatesAMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	changed, err := ApplyKeepalive(path, "github.com")
	if err != nil {
		t.Fatalf("ApplyKeepalive: %v", err)
	}
	if !changed {
		t.Error("want changed=true when creating the file")
	}
	if !strings.Contains(readFile(t, path), "ServerAliveInterval") {
		t.Error("created config lacks the keepalive")
	}
	// ssh refuses to use a group/world-readable config for some settings, and a
	// file we create should not be the reason a user's ssh starts complaining.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("created config mode = %o, want 600", perm)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
