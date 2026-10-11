package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parsePowerSave reads `iw dev <d> get power_save`. "on"/"off" are facts; any
// other answer is "don't know" (ok=false) and must never raise a false alarm.
func TestParsePowerSave(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		wantOn  bool
		wantsOK bool
	}{
		{"on", "Power save: on\n", true, true},
		{"off", "\tPower save: off\n", false, true},
		{"tabbed-on", "\tPower save: on", true, true},
		{"mixed-case", "POWER SAVE: ON", true, true},
		{"unexpected", "command not found", false, false},
		// The marker must BE the line, not appear inside other text.
		{"embedded", "warning: expected Power save: on here\n", false, false},
		{"garbled value", "Power save: maybe\n", false, false},
		{"empty", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			on, ok := parsePowerSave(c.out)
			if on != c.wantOn || ok != c.wantsOK {
				t.Fatalf("parsePowerSave(%q) = (%v,%v), want (%v,%v)", c.out, on, ok, c.wantOn, c.wantsOK)
			}
		})
	}
}

// powerSaveNote speaks only when a named interface is on. Off, or no interface,
// is silence — the common case must add nothing to the output.
func TestPowerSaveNoteOnlyFiresWhenARadioIsAsleep(t *testing.T) {
	if got := powerSaveNote("wlan0", false); got != "" {
		t.Fatalf("power save off must be silent, got:\n%s", got)
	}
	if got := powerSaveNote("", true); got != "" {
		t.Fatalf("no interface must be silent, got:\n%s", got)
	}
	got := powerSaveNote("wlan0", true)
	for _, want := range []string{"wlan0", "power_save off", "wifi.powersave = 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("note missing %q:\n%s", want, got)
		}
	}
}

// iw lives in /usr/sbin, which a normal user's PATH lacks on Raspberry Pi OS
// and Debian, so a bare-name lookup never fired on the platform this targets.
func TestFindIWFallsBackToTheSbinDirs(t *testing.T) {
	noPath := func(string) (string, error) { return "", errors.New("not in PATH") }
	onPath := func(string) (string, error) { return "/opt/x/iw", nil }
	only := func(p string) func(string) bool { return func(q string) bool { return q == p } }
	if got := findIW(onPath, only("")); got != "/opt/x/iw" {
		t.Fatalf("PATH hit = %q", got)
	}
	if got := findIW(noPath, only("/usr/sbin/iw")); got != "/usr/sbin/iw" {
		t.Fatalf("usr/sbin fallback = %q", got)
	}
	if got := findIW(noPath, only("/sbin/iw")); got != "/sbin/iw" {
		t.Fatalf("sbin fallback = %q", got)
	}
	if got := findIW(noPath, only("")); got != "" {
		t.Fatalf("no iw anywhere = %q", got)
	}
}

func fakeNIC(t *testing.T, root, name string, wireless bool, oper string) {
	t.Helper()
	d := filepath.Join(root, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if wireless {
		if err := os.MkdirAll(filepath.Join(d, "wireless"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(d, "operstate"), []byte(oper+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Only an interface that is UP can be the reason the fleet looks unreachable:
// an idle dongle with power save on is not on the path to anything.
func TestRadioProbeReportsOnlyAnUpWirelessNICWithPowerSaveOn(t *testing.T) {
	root := t.TempDir()
	fakeNIC(t, root, "eth0", false, "up")
	fakeNIC(t, root, "wlan1", true, "down") // idle dongle, power save on
	fakeNIC(t, root, "wlan0", true, "up")
	var asked []string
	run := func(ctx context.Context, iw, iface string) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("iw must run under a deadline")
		}
		asked = append(asked, iface)
		return "\tPower save: on\n", nil
	}
	p := radioProbe{sysNet: root, iw: "/usr/sbin/iw", run: run}
	iface, on := p.powerSaveOn()
	if iface != "wlan0" || !on {
		t.Fatalf("got (%q,%v), want (wlan0,true)", iface, on)
	}
	if strings.Join(asked, ",") != "wlan0" {
		t.Fatalf("iw asked about %v; only the up wireless NIC should be probed", asked)
	}
}

func TestRadioProbeIsSilentWithoutIWOrOnError(t *testing.T) {
	root := t.TempDir()
	fakeNIC(t, root, "wlan0", true, "up")
	never := func(context.Context, string, string) (string, error) {
		t.Fatal("iw must not run when it was not found")
		return "", nil
	}
	if _, on := (radioProbe{sysNet: root, iw: "", run: never}).powerSaveOn(); on {
		t.Fatal("no iw must be silent")
	}
	fail := func(context.Context, string, string) (string, error) {
		return "Power save: on", errors.New("exit status 1")
	}
	if _, on := (radioProbe{sysNet: root, iw: "/sbin/iw", run: fail}).powerSaveOn(); on {
		t.Fatal("a failing iw must never raise the note")
	}
	if _, on := (radioProbe{sysNet: filepath.Join(root, "absent"), iw: "/sbin/iw", run: fail}).powerSaveOn(); on {
		t.Fatal("no /sys/class/net (macOS) must be silent")
	}
}
