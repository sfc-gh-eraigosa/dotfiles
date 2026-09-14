package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// Owner-reported: a host without NOPASSWD dropped out of the streaming TUI
// into the interactive lane and asked for the password AGAIN, though it had
// been typed into the form. Over ssh there is no tty, so sudo keys its cached
// credential to the PPID that primed it and install.sh's children never see
// it. The fix the security review ranked first: with the operator's opt-in
// (gff fleet.update.sudo-timestamp-global), the preamble installs
// /etc/sudoers.d/fleet-timestamp (Defaults:<user> timestamp_type=global) using
// the credential it just primed — which DOES work from the priming shell —
// re-primes, and the host stays in the background lane. The password never
// leaves the priming shell (a shell variable, expanded only into a pipe to
// sudo -S, with xtrace off); children gain root for the sudo timeout on THAT
// host, never the password itself.

// fakeSudoGlobal models sudo without NOPASSWD and without a tty: `-S -v`
// reads the password from stdin and caches it for the caller's PPID; a bare
// `sudo cmd` (or `-n`) works from that PPID only — unless a file under
// FAKE_SUDOERS_D says timestamp_type=global, when any cached prime serves
// every PPID. `visudo -cf` validates the drop-in's shape; `install ... SRC
// /etc/sudoers.d/X` copies SRC into FAKE_SUDOERS_D. FAKE_PW is the password.
func fakeSudoGlobal(t *testing.T, dir string) {
	t.Helper()
	script := `#!/bin/sh
D="` + dir + `"
global() { grep -qs 'timestamp_type=global' "$FAKE_SUDOERS_D"/* 2>/dev/null; }
cached() { if global; then ls "$D"/primed.* >/dev/null 2>&1; else [ -f "$D/primed.$PPID" ]; fi; }
case "$1" in
  -S) IFS= read -r pw; [ "$pw" = "$FAKE_PW" ] || exit 1; touch "$D/primed.$PPID"; exit 0 ;;
  -k) exit 0 ;;
  -n) shift; cached || { echo "sudo: a password is required" >&2; exit 1; }; exit 0 ;;
  visudo) cached || exit 1; [ -z "${FAKE_VISUDO_FAIL:-}" ] || exit 1
      grep -Eq '^Defaults:[A-Za-z_][A-Za-z0-9_-]* timestamp_type=global$' "$3"; exit $? ;;
  install) cached || exit 1
      dst=$(printf '%s' "$7" | sed "s#^/etc/sudoers.d#$FAKE_SUDOERS_D#"); cp "$6" "$dst" && chmod "$3" "$dst"; exit $? ;;
  *) cached || { echo "sudo: a terminal is required to read the password" >&2; exit 1; }; echo "$*" >> "$D/ran"; exit 0 ;;
esac
`
	if err := os.WriteFile(dir+"/sudo", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

type sudoWorld struct {
	sudoDir, tmp, sudoersD string
}

func newSudoWorld(t *testing.T) sudoWorld {
	t.Helper()
	w := sudoWorld{sudoDir: t.TempDir(), tmp: t.TempDir(), sudoersD: t.TempDir()}
	fakeSudoGlobal(t, w.sudoDir)
	return w
}

// run executes the background-lane preamble plus probe under sh, with the
// password on stdin exactly as the runner sends it.
func (w sudoWorld) run(t *testing.T, sh, stdin, probe string, a answers, p bgPolicy, extraEnv ...string) (string, error) {
	t.Helper()
	script := bgPreambleWith(a, p)(updplan.Step{Kind: updplan.KindRun}) + probe
	c := exec.Command(sh, "-c", script)
	c.Env = append([]string{"PATH=" + w.sudoDir + ":/usr/bin:/bin", "TMPDIR=" + w.tmp, "HOME=" + w.tmp,
		"FAKE_PW=" + probeMarker, "FAKE_SUDOERS_D=" + w.sudoersD}, extraEnv...)
	c.Stdin = strings.NewReader(stdin + "\n")
	out, err := c.CombinedOutput()
	return string(out), err
}

func (w sudoWorld) dropIn() string {
	b, _ := os.ReadFile(w.sudoersD + "/fleet-timestamp")
	return string(b)
}

const childProbe = `sh -c 'sh -c "sudo apt-get install -y x"' && echo children-ok`

func TestSudoGlobalOptInInstallsTheDropInAndStaysInTheBackgroundLane(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	on := bgPolicy{sudoTimestampGlobal: true}
	for _, sh := range []string{"/bin/bash", "dash", "zsh"} {
		path, err := exec.LookPath(sh)
		if err != nil {
			t.Logf("skip %s: not installed", sh)
			continue
		}
		w := newSudoWorld(t)
		out, err := w.run(t, path, probeMarker, childProbe, a, on)
		if err != nil || !strings.Contains(out, "children-ok") {
			t.Fatalf("%s: with the opt-in, install.sh's children must sudo and the run must go on: err=%v\n%s", sh, err, out)
		}
		want := "Defaults:" + currentUser(t) + " timestamp_type=global\n"
		if got := w.dropIn(); got != want {
			t.Fatalf("%s: drop-in must be exactly %q, got %q", sh, want, got)
		}
		if !strings.Contains(out, "installed /etc/sudoers.d/fleet-timestamp") {
			t.Fatalf("%s: the run must SAY it changed the host:\n%s", sh, out)
		}
		if left, _ := os.ReadDir(w.tmp); len(left) != 0 {
			t.Fatalf("%s: the temp sudoers file must be removed, left %d entries", sh, len(left))
		}
		if strings.Contains(out, probeMarker) {
			t.Fatalf("%s: the password leaked into the output:\n%s", sh, out)
		}
	}
}

func TestSudoGlobalOffKeepsTodaysReroute(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, a, bgPolicy{})
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != rcSudoNoCache {
		t.Fatalf("flag off: the gate must still exit %d so the host is rerouted, got err=%v out=%q", rcSudoNoCache, err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("flag off: the host must not be changed")
	}
}

func TestSudoGlobalNeverWritesWhenTheCredentialAlreadyReachesChildren(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	w := newSudoWorld(t)
	// A host already configured (or NOPASSWD): the gate passes first time.
	if err := os.WriteFile(w.sudoersD+"/site", []byte("Defaults timestamp_type=global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, a, bgPolicy{sudoTimestampGlobal: true})
	if err != nil || !strings.Contains(out, "children-ok") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if w.dropIn() != "" || strings.Contains(out, "installed") {
		t.Fatalf("nothing to fix, so nothing may be written or announced:\n%s", out)
	}
}

func TestSudoGlobalRejectedPasswordWritesNothing(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", "wrong", "echo must-not-run", a, bgPolicy{sudoTimestampGlobal: true})
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != rcSudoAuth || strings.Contains(out, "must-not-run") {
		t.Fatalf("a rejected password must exit %d before anything else: err=%v out=%q", rcSudoAuth, err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("a rejected password must not touch sudoers")
	}
}

func TestSudoGlobalVisudoRejectionFallsBackToTheReroute(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, a, bgPolicy{sudoTimestampGlobal: true}, "FAKE_VISUDO_FAIL=1")
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != rcSudoNoCache {
		t.Fatalf("when visudo refuses the drop-in nothing is installed and the gate reroutes (92): err=%v out=%q", err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("an invalid drop-in must never be installed")
	}
}

func TestSudoGlobalPreambleShape(t *testing.T) {
	a := answers{}
	a.appendSecret(probeMarker)
	off := bgPreambleWith(a, bgPolicy{})(updplan.Step{Kind: updplan.KindRun})
	if off != bgPreamble(a)(updplan.Step{Kind: updplan.KindRun}) || strings.Contains(off, "sudoers") {
		t.Fatalf("flag off must be exactly today's preamble:\n%s", off)
	}
	on := bgPreambleWith(a, bgPolicy{sudoTimestampGlobal: true})(updplan.Step{Kind: updplan.KindRun})
	for _, want := range []string{"set +x", "visudo -cf", "install -m 0440", sudoersDropIn, "timestamp_type=global", "sudo -S -p '' -v"} {
		if !strings.Contains(on, want) {
			t.Fatalf("opt-in preamble missing %q:\n%s", want, on)
		}
	}
	if strings.Contains(on, probeMarker) {
		t.Fatalf("the password leaked into the remote command:\n%s", on)
	}
	// Order: prime, then the fixup guarded by a first child check, then the
	// gate that decides the lane.
	if i, j := strings.Index(on, "visudo"), strings.LastIndex(on, "sudo -n true"); i < 0 || j < 0 || i > j {
		t.Fatalf("the fixup must run before the deciding gate:\n%s", on)
	}
	// No credential: nothing to prime with, so nothing to fix — no sudoers logic.
	none := bgPreambleWith(answers{}, bgPolicy{sudoTimestampGlobal: true})(updplan.Step{Kind: updplan.KindRun})
	if strings.Contains(none, "sudoers") || strings.Contains(none, "sudo -S") {
		t.Fatalf("no credential means no fixup:\n%s", none)
	}
}

func TestBgPolicyComesFromTheGffFlag(t *testing.T) {
	on := bgPolicyFromFlags(featflag.Static{Bools: map[string]bool{featflag.KeySudoGlobal: true}}, t.TempDir())
	if !on.sudoTimestampGlobal {
		t.Fatal("an explicit true must opt the lane in")
	}
	off := bgPolicyFromFlags(featflag.Static{Bools: map[string]bool{featflag.KeyEnabled: true}}, t.TempDir())
	if off.sudoTimestampGlobal {
		t.Fatal("a missing key must stay off (fail-closed)")
	}
	if bgPolicyFromFlags(nil, "").sudoTimestampGlobal {
		t.Fatal("no gff at all must stay off")
	}
}

func currentUser(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
