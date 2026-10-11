package pins

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// fakeExec answers by EXACT probe string. A probe with no entry behaves like a
// missing command (exit 127, "not found"), so a typo in a test fails loudly.
type fakeExec map[string]struct {
	out string
	err error
}

func (f fakeExec) run(_ context.Context, cmd string) (string, error) {
	if r, ok := f[cmd]; ok {
		return r.out, r.err
	}
	return "sh: 1: " + cmd + ": not found", errors.New("exit status 127")
}

func ok(out string) struct {
	out string
	err error
} {
	return struct {
		out string
		err error
	}{out, nil}
}

const installedManifest = `
a-pin     pin    1.2.3   -  -  a --version
b-floor   floor  1.96.0  -  -  b --version
c-latest  latest -       -  -  c --version
d-none    pin    2.0.0   -  -  -
e-miss    pin    3.0.0   -  -  e --version
f-junk    pin    4.0.0   -  -  f --version
g-empty   pin    5.0.0   -  -  g --version
h-fail    pin    6.0.0   -  -  h --version
`

func TestCheckInstalledStates(t *testing.T) {
	m, err := Parse(strings.NewReader(installedManifest), t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	ex := fakeExec{
		"a --version": ok("a v1.2.0 (abc)"),
		"b --version": ok("rustc 1.98.1 (x 2026-09-01)"),
		"c --version": ok("c 9.9.9"),
		"f --version": ok("version is set by build process"),
		"g --version": ok("   \n"),
		"h --version": {out: "h 5.9.9\n", err: errors.New("exit status 1")}, // exits non-zero but printed a version
	}
	rows, sum := CheckInstalled(context.Background(), m, ex.run, "manifest")
	want := []InstalledRow{
		{"a-pin", "pin", "1.2.3", "1.2.0", "drift"},
		{"b-floor", "floor", "1.96.0", "1.98.1", "ok"},
		{"c-latest", "latest", "latest", "9.9.9", "ok"},
		{"d-none", "pin", "2.0.0", "", "unknown"},
		{"e-miss", "pin", "3.0.0", "", "missing"},
		{"f-junk", "pin", "4.0.0", "", "unknown"},
		{"g-empty", "pin", "5.0.0", "", "missing"},
		{"h-fail", "pin", "6.0.0", "5.9.9", "drift"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d", len(rows))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
	if sum != (Summary{OK: 2, Drift: 2, Missing: 2, Unknown: 2}) {
		t.Errorf("summary = %+v", sum)
	}
}

func TestCheckInstalledPinEqualAndFloorBelow(t *testing.T) {
	m, _ := Parse(strings.NewReader("a pin 1.2.3 - - a\nb floor 1.97.0 - - b\n"), t.TempDir(), "x")
	ex := fakeExec{"a": ok("1.2.3"), "b": ok("1.96.9")}
	rows, sum := CheckInstalled(context.Background(), m, ex.run, "")
	if rows[0].State != "ok" || rows[1].State != "drift" || sum.OK != 1 || sum.Drift != 1 {
		t.Errorf("rows=%+v sum=%+v", rows, sum)
	}
}

func TestCheckInstalledModes(t *testing.T) {
	m, _ := Parse(strings.NewReader("a pin 1.2.3 - - a\nb floor 1.0.0 - - b\nc latest - - - c\n"), t.TempDir(), "x")
	ex := fakeExec{"a": ok("a 1.0.0"), "b": ok("b 1.5.0"), "c": ok("c 7.0.0")}

	rows, _ := CheckInstalled(context.Background(), m, ex.run, "latest")
	for _, r := range rows {
		if r.Want != "latest" || r.State != "ok" {
			t.Errorf("latest mode row = %+v", r)
		}
	}
	if rows[0].Installed != "1.0.0" {
		t.Errorf("latest mode must still report installed: %+v", rows[0])
	}

	rows, _ = CheckInstalled(context.Background(), m, ex.run, "pin")
	if rows[0].State != "drift" || rows[1].State != "drift" || rows[2].Want != "latest" || rows[2].State != "ok" {
		t.Errorf("pin mode rows = %+v", rows)
	}

	// latest mode still reports a missing tool as missing.
	rows, _ = CheckInstalled(context.Background(), m, fakeExec{}.run, "latest")
	if rows[0].State != "missing" {
		t.Errorf("latest + absent = %+v", rows[0])
	}
}

func TestCheckInstalledPerRowTimeout(t *testing.T) {
	m, _ := Parse(strings.NewReader("a pin 1.0.0 - - a\n"), t.TempDir(), "x")
	var deadline time.Time
	ex := func(ctx context.Context, _ string) (string, error) {
		deadline, _ = ctx.Deadline()
		<-ctx.Done()
		return "", ctx.Err()
	}
	old := probeTimeout
	probeTimeout = 50 * time.Millisecond
	defer func() { probeTimeout = old }()
	rows, sum := CheckInstalled(context.Background(), m, ex, "manifest")
	if deadline.IsZero() || rows[0].State != "unknown" || sum.Unknown != 1 {
		t.Errorf("rows=%+v sum=%+v deadline=%v", rows, sum, deadline)
	}
	if probeTimeout != 50*time.Millisecond || old != 5*time.Second {
		t.Errorf("default probe timeout = %v, want 5s", old)
	}
}

func TestShellExec(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	out, err := ShellExec(context.Background(), "echo hi; echo err 1>&2")
	if err != nil || !strings.Contains(out, "hi") || !strings.Contains(out, "err") {
		t.Errorf("out=%q err=%v", out, err)
	}
	if _, err := ShellExec(context.Background(), "exit 3"); err == nil {
		t.Error("want exit error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := ShellExec(ctx, "sleep 5"); err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("timeout not enforced: err=%v after %v", err, time.Since(start))
	}
}
