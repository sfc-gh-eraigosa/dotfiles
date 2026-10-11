package pins

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Review fixes on the pins branch (see docs/mbo/plans/pins/TRACKING.md §4).

func TestProbePathPutsTheUserToolDirsFirst(t *testing.T) {
	got := ProbePath("/h", "/usr/bin:/bin")
	for _, want := range []string{"/h/opt/bin", "/h/.local/bin", "/h/.cargo/bin", "/h/.goenv/shims"} {
		if !strings.Contains(got, want+":") {
			t.Fatalf("ProbePath = %q lacks %q", got, want)
		}
	}
	if !strings.HasSuffix(got, ":/usr/bin:/bin") {
		t.Fatalf("ProbePath must keep the inherited PATH last: %q", got)
	}
	// A non-login ssh shell has no HOME-relative dirs at all; with no home we
	// must not invent "/opt/bin".
	if ProbePath("", "/usr/bin") != "/usr/bin" {
		t.Fatalf("no home must leave PATH alone")
	}
}

func TestFailedProbeIsNotTrustedEvenWhenItQuotesAVersion(t *testing.T) {
	// goenv: version '1.26.3' is not installed — the wanted version appears in
	// the ERROR, which must not read as "1.26.3 installed".
	m, _ := Parse(strings.NewReader("go pin 1.26.3 - - go version\n"), t.TempDir(), "x")
	ex := func(context.Context, string) (string, error) {
		return "goenv: version '1.26.3' is not installed (set by /r/.go-version)", errors.New("exit status 1")
	}
	rows, _ := CheckInstalled(context.Background(), m, ex, "manifest")
	if rows[0].State == "ok" || rows[0].Installed == "1.26.3" {
		t.Fatalf("a failing probe was trusted: %+v", rows[0])
	}
}

func TestPinModeHoldsALatestRowToItsConcreteVersion(t *testing.T) {
	m, _ := Parse(strings.NewReader("foo latest 5.0.0 - - foo\nbar latest - - - bar\n"), t.TempDir(), "x")
	ex := func(_ context.Context, cmd string) (string, error) { return cmd + " 6.1.0", nil }
	rows, _ := CheckInstalled(context.Background(), m, ex, "pin")
	if rows[0].Want != "5.0.0" || rows[0].State != "drift" {
		t.Fatalf("pin mode, latest row with a version: %+v", rows[0])
	}
	// No concrete version to hold it to: it still floats.
	if rows[1].Want != "latest" || rows[1].State != "ok" {
		t.Fatalf("pin mode, latest row without a version: %+v", rows[1])
	}
}
