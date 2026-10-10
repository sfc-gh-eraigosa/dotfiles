package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// fakeKnownHosts simulates ssh-keygen / ssh-keyscan over a real temp file, so
// a test can assert on what known_hosts holds afterwards rather than on which
// commands were issued.
type fakeKnownHosts struct {
	t        *testing.T
	path     string
	removals int
}

func newFakeKnownHosts(t *testing.T, content string) *fakeKnownHosts {
	p := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return &fakeKnownHosts{t: t, path: p}
}

func (f *fakeKnownHosts) read() string { b, _ := os.ReadFile(f.path); return string(b) }

func (f *fakeKnownHosts) exec(name, stdin string, args ...string) (string, error) {
	switch {
	case name == "ssh" && args[0] == "-G":
		return "hostname 10.0.0.9\nport 22\nhashknownhosts no\nuserknownhostsfile " + f.path + "\n", nil
	case name == "ssh-keygen" && args[0] == "-F":
		var hit []string
		for _, l := range strings.Split(f.read(), "\n") {
			if strings.HasPrefix(l, args[1]+" ") {
				hit = append(hit, l)
			}
		}
		if len(hit) == 0 {
			return "", errors.New("exit status 1")
		}
		return "# Host " + args[1] + " found: line 1\n" + strings.Join(hit, "\n") + "\n", nil
	case name == "ssh-keygen" && args[0] == "-lf":
		var out []string
		if strings.Contains(stdin, "OLDKEY") {
			out = append(out, "256 SHA256:old 10.0.0.9 (ECDSA)")
		}
		if strings.Contains(stdin, "NEWKEY") {
			out = append(out, "256 SHA256:new 10.0.0.9 (ED25519)")
		}
		return strings.Join(out, "\n") + "\n", nil
	case name == "ssh-keyscan":
		return "10.0.0.9 ssh-ed25519 NEWKEY\n", nil
	case name == "ssh-keygen" && args[0] == "-R":
		f.removals++
		var keep []string
		for _, l := range strings.Split(f.read(), "\n") {
			if l != "" && !strings.HasPrefix(l, args[1]+" ") {
				keep = append(keep, l+"\n")
			}
		}
		return "", os.WriteFile(f.path, []byte(strings.Join(keep, "")), 0o600)
	}
	f.t.Fatalf("unexpected command %s %v", name, args)
	return "", nil
}

type trustRun struct {
	out     bytes.Buffer
	audited []map[string]any
	err     error
}

func runTrustWith(t *testing.T, kh *fakeKnownHosts, stdin, pin string, r runner.Runner) *trustRun {
	t.Helper()
	tr := &trustRun{}
	tr.err = runTrust(trustOpts{
		Alias: "gig",
		Pin:   pin,
		In:    strings.NewReader(stdin),
		Out:   &tr.out,
		X:     kh.exec,
		R:     r,
		Now:   time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC),
		Audit: func(f map[string]any) { tr.audited = append(tr.audited, f) },
	})
	return tr
}

const otherHost = "10.0.0.5 ssh-ed25519 UNRELATED\n"

func TestTrustShowsBothFingerprintsAndRefusesWithoutTheTypedAlias(t *testing.T) {
	kh := newFakeKnownHosts(t, otherHost+"10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "y\n", "", runner.Fake{})

	if tr.err == nil {
		t.Fatal("a bare y must not accept a changed host key")
	}
	for _, want := range []string{"CHANGED", "SHA256:old", "SHA256:new", "/etc/ssh/ssh_host_*_key.pub"} {
		if !strings.Contains(tr.out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, tr.out.String())
		}
	}
	if got := kh.read(); !strings.Contains(got, "OLDKEY") || strings.Contains(got, "NEWKEY") {
		t.Fatalf("known_hosts changed without confirmation:\n%s", got)
	}
	if len(tr.audited) != 0 {
		t.Fatal("nothing changed, so nothing may be recorded as trusted")
	}
}

func TestTrustReplacesTheKeyOnlyAfterTheAliasIsTyped(t *testing.T) {
	kh := newFakeKnownHosts(t, otherHost+"10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "gig\n", "", runner.Fake{Out: map[string]string{"gig": ""}})

	if tr.err != nil {
		t.Fatalf("err: %v\n%s", tr.err, tr.out.String())
	}
	got := kh.read()
	if strings.Contains(got, "OLDKEY") || !strings.Contains(got, "NEWKEY") || !strings.Contains(got, "UNRELATED") {
		t.Fatalf("known_hosts after trust:\n%s", got)
	}
	backup := kh.path + ".fleet-bak-20261010T150000Z"
	if b, err := os.ReadFile(backup); err != nil || !strings.Contains(string(b), "OLDKEY") {
		t.Fatalf("backup %s: %v %q", backup, err, b)
	}
	if !strings.Contains(tr.out.String(), backup) {
		t.Errorf("the backup path must be printed so the change can be undone:\n%s", tr.out.String())
	}
}

func TestTrustRecordsTheReplacementForLaterReview(t *testing.T) {
	kh := newFakeKnownHosts(t, "10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "gig\n", "", runner.Fake{Out: map[string]string{"gig": ""}})
	if tr.err != nil {
		t.Fatal(tr.err)
	}
	if len(tr.audited) != 1 {
		t.Fatalf("want one audit record, got %d", len(tr.audited))
	}
	a := tr.audited[0]
	if a["host"] != "gig" || a["verdict"] != "changed" ||
		!strings.Contains(a["old"].(string), "SHA256:old") || !strings.Contains(a["new"].(string), "SHA256:new") {
		t.Fatalf("audit record %v", a)
	}
}

func TestTrustPinThatDoesNotMatchChangesNothing(t *testing.T) {
	kh := newFakeKnownHosts(t, "10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "", "SHA256:somethingelse", runner.Fake{})
	if tr.err == nil || kh.removals != 0 || !strings.Contains(kh.read(), "OLDKEY") {
		t.Fatalf("a mismatched --fingerprint must refuse: err=%v removals=%d", tr.err, kh.removals)
	}
}

func TestTrustMatchingPinNeedsNoPrompt(t *testing.T) {
	kh := newFakeKnownHosts(t, "10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "", "SHA256:new", runner.Fake{Out: map[string]string{"gig": ""}})
	if tr.err != nil || !strings.Contains(kh.read(), "NEWKEY") {
		t.Fatalf("err=%v known_hosts=%q", tr.err, kh.read())
	}
}

func TestTrustOnAnAlreadyTrustedKeyIsANoop(t *testing.T) {
	kh := newFakeKnownHosts(t, "10.0.0.9 ssh-ed25519 NEWKEY\n")
	tr := runTrustWith(t, kh, "", "", runner.Fake{Out: map[string]string{"gig": ""}})
	if tr.err != nil || kh.removals != 0 || len(tr.audited) != 0 {
		t.Fatalf("err=%v removals=%d audited=%d", tr.err, kh.removals, len(tr.audited))
	}
	if !strings.Contains(tr.out.String(), "already trusted") {
		t.Fatalf("output:\n%s", tr.out.String())
	}
}

func TestTrustFirstContactIsNotCalledAChange(t *testing.T) {
	kh := newFakeKnownHosts(t, otherHost)
	tr := runTrustWith(t, kh, "gig\n", "", runner.Fake{Out: map[string]string{"gig": ""}})
	if tr.err != nil {
		t.Fatal(tr.err)
	}
	if strings.Contains(tr.out.String(), "CHANGED") {
		t.Fatalf("an unknown host is first contact, not a MITM warning:\n%s", tr.out.String())
	}
	if !strings.Contains(kh.read(), "NEWKEY") {
		t.Fatal("first contact should still record the key")
	}
}

func TestTrustReportsWhenTheHostStillRefusesAfterwards(t *testing.T) {
	kh := newFakeKnownHosts(t, "10.0.0.9 ecdsa-sha2-nistp256 OLDKEY\n")
	tr := runTrustWith(t, kh, "gig\n", "", runner.Fake{Err: map[string]error{"gig": errors.New("exit status 255")}})
	if tr.err == nil {
		t.Fatal("a host that still refuses after the swap is not fixed")
	}
}
