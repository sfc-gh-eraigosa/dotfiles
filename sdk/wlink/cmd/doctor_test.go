package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSSHCmd struct {
	out   string
	calls int
}

func (f *fakeSSHCmd) Run(context.Context, string, ...string) ([]byte, error) {
	f.calls++
	return []byte(f.out), nil
}

func doctorRuntime(t *testing.T, sshOut string) (*Runtime, *strings.Builder, *fakeSSHCmd) {
	t.Helper()
	rt, _ := healthyRuntime(t)
	out := &strings.Builder{}
	rt.Out = out
	f := &fakeSSHCmd{out: sshOut}
	rt.SSHCmd = f
	rt.SSHConfigPath = filepath.Join(t.TempDir(), "config")
	return rt, out, f
}

// EC-10: the finding that came from a real incident — git hanging forever over
// a tunnel because ssh never notices a stalled connection.
func TestDoctor_FlagsDisabledKeepalive(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "serveraliveinterval 0\nconnecttimeout none\n")
	code, err := rt.Doctor(context.Background(), false)
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 (a finding is present)", code)
	}
	s := out.String()
	if !strings.Contains(s, "keepalive") {
		t.Errorf("keepalive not flagged:\n%s", s)
	}
	if !strings.Contains(s, "hangs forever") {
		t.Errorf("finding must explain the consequence, not just the setting:\n%s", s)
	}
	if !strings.Contains(s, "--fix") {
		t.Errorf("finding must name the remedy:\n%s", s)
	}
}

// Silent when it is already set: a checker that warns about a healthy machine
// trains people to ignore it.
func TestDoctor_SilentWhenKeepaliveIsSet(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "serveraliveinterval 20\nconnecttimeout 10\n")
	if _, err := rt.Doctor(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "[warn] ssh has no keepalive") {
		t.Errorf("warned about a configured keepalive:\n%s", out)
	}
}

// --fix must verify with ssh rather than assume: ssh takes the FIRST value it
// sees, so an earlier Host block can render the appended one inert.
func TestDoctor_FixVerifiesRatherThanAssuming(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "serveraliveinterval 0\nconnecttimeout none\n") // still 0 after the fix
	if _, err := rt.Doctor(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Contains(s, "[ok]   ssh keepalive") && strings.Contains(s, "applied") {
		t.Errorf("claimed success while ssh still reports it disabled:\n%s", s)
	}
	if !strings.Contains(s, "wins") {
		t.Errorf("should explain that an earlier block wins:\n%s", s)
	}
	if _, err := os.Stat(rt.SSHConfigPath); err != nil {
		t.Errorf("--fix did not write the config: %v", err)
	}
}

// Cannot check is not the same as broken.
func TestDoctor_MissingSSHIsNotAFinding(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "")
	rt.SSHCmd = errSSH{}
	if _, err := rt.Doctor(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "[warn] ssh has no keepalive") {
		t.Errorf("reported a finding it could not actually check:\n%s", out)
	}
}

type errSSH struct{}

func (errSSH) Run(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrNotExist }

// A machine with no fleet names cannot be checked at all, which is worth saying.
func TestDoctor_FlagsAnEmptyFleet(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "serveraliveinterval 20\n")
	rt.FleetHosts, rt.ExcludedHosts = nil, nil
	code, err := rt.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "no fleet names") {
		t.Errorf("empty fleet not reported:\n%s", out)
	}
}

func TestDoctor_NonWSLIsANoOp(t *testing.T) {
	rt, out, _ := doctorRuntime(t, "")
	rt.WSL = false
	code, err := rt.Doctor(context.Background(), false)
	if err != nil || code != 0 {
		t.Errorf("Doctor off WSL = (%d, %v), want (0, nil)", code, err)
	}
	if out.Len() == 0 {
		t.Error("must still say why it did nothing")
	}
}
