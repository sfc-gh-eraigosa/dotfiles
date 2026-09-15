package cmd

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// Owner-reported: a host without NOPASSWD dropped out of the streaming TUI
// into the interactive lane and asked for the password AGAIN, though it had
// been typed into the form. Over ssh there is no tty, so sudo keys its cached
// credential to the PPID that primed it and install.sh's children never see
// it. The fix the security review ranked first: unless the operator opted out
// (gff fleet.update.sudo-timestamp-global), the preamble installs
// /etc/sudoers.d/fleet-timestamp (Defaults:<user> timestamp_type=global) using
// the credential it just primed — which DOES work from the priming shell —
// re-primes, and the host stays in the background lane. The password never
// leaves the priming shell (a shell variable, expanded only into a pipe to
// sudo -S, with xtrace off); children gain root for the sudo timeout on THAT
// host, never the password itself.

// fakeSudoGlobal models sudo without NOPASSWD and without a tty: `-S -v`
// reads the password from stdin and caches it for the caller's PPID; a bare
// `sudo cmd` (or `-n cmd`) works from that PPID only — unless a file under
// FAKE_SUDOERS_D says timestamp_type=global, when any cached prime serves
// every PPID (FAKE_INERT=1 models a sudoers that never includes sudoers.d:
// the drop-in is installed but changes nothing). `-n sh -c SCRIPT` runs
// SCRIPT the way root's fresh sh would, with fake `visudo` and `install`
// ahead on PATH: `visudo -cf FILE` validates the drop-in's shape (or refuses
// under FAKE_VISUDO_FAIL), `install ... SRC /etc/sudoers.d/X` copies SRC into
// FAKE_SUDOERS_D, and `-n rm -f /etc/sudoers.d/X` removes it from there.
// FAKE_PW is the password; FAKE_DIR holds the PPID cache.
func fakeSudoGlobal(t *testing.T, dir, rootBin string) {
	t.Helper()
	sudo := `#!/bin/sh
D="$FAKE_DIR"
global() { [ -z "${FAKE_INERT:-}" ] && grep -qs 'timestamp_type=global' "$FAKE_SUDOERS_D"/* 2>/dev/null; }
cached() { if global; then ls "$D"/primed.* >/dev/null 2>&1; else [ -f "$D/primed.$PPID" ]; fi; }
map() { printf '%s' "$1" | sed "s#^/etc/sudoers.d#$FAKE_SUDOERS_D#"; }
case "$1" in
  -S) IFS= read -r pw; [ "$pw" = "$FAKE_PW" ] || exit 1; touch "$D/primed.$PPID"; exit 0 ;;
  -n) shift; cached || { echo "sudo: a password is required" >&2; exit 1; }
      case "$1" in
        sh) shift; PATH="$FAKE_ROOTBIN:$PATH" exec sh "$@" ;;
        rm) rm -f "$(map "$3")"; exit $? ;;
        *) exit 0 ;;
      esac ;;
  *) cached || { echo "sudo: a terminal is required to read the password" >&2; exit 1; }; echo "$*" >> "$D/ran"; exit 0 ;;
esac
`
	visudo := `#!/bin/sh
[ "$1" = "-cf" ] || exit 2
[ -z "${FAKE_VISUDO_FAIL:-}" ] || { echo "visudo: >>> $2: unknown defaults entry \"timestamp_type\" <<<" >&2; exit 1; }
grep -Eq '^Defaults:[^ ]+ timestamp_type=global$' "$2" || exit 1
echo "$2: parsed OK"
`
	install := `#!/bin/sh
while [ $# -gt 2 ]; do shift; done
cp "$1" "$(printf '%s' "$2" | sed "s#^/etc/sudoers.d#$FAKE_SUDOERS_D#")"
`
	for path, body := range map[string]string{dir + "/sudo": sudo, rootBin + "/visudo": visudo, rootBin + "/install": install} {
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

type sudoWorld struct {
	sudoDir, rootBin, tmp, sudoersD string
}

func newSudoWorld(t *testing.T) sudoWorld {
	t.Helper()
	w := sudoWorld{sudoDir: t.TempDir(), rootBin: t.TempDir(), tmp: t.TempDir(), sudoersD: t.TempDir()}
	fakeSudoGlobal(t, w.sudoDir, w.rootBin)
	return w
}

// run executes the background-lane preamble plus probe under sh, with the
// password on stdin exactly as the runner sends it.
func (w sudoWorld) run(t *testing.T, sh, stdin, probe string, a answers, p bgPolicy, extraEnv ...string) (string, error) {
	t.Helper()
	script := bgPreamble(a, p)(updplan.Step{Kind: updplan.KindRun}) + probe
	c := exec.Command(sh, "-c", script)
	c.Env = append([]string{"PATH=" + w.sudoDir + ":/usr/bin:/bin", "TMPDIR=" + w.tmp, "HOME=" + w.tmp,
		"FAKE_PW=" + probeMarker, "FAKE_DIR=" + w.sudoDir, "FAKE_ROOTBIN=" + w.rootBin, "FAKE_SUDOERS_D=" + w.sudoersD}, extraEnv...)
	c.Stdin = strings.NewReader(stdin + "\n")
	out, err := c.CombinedOutput()
	return string(out), err
}

func (w sudoWorld) dropIn() string {
	b, _ := os.ReadFile(w.sudoersD + "/fleet-timestamp")
	return string(b)
}

func shExit(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func secretAnswers() answers {
	a := answers{}
	a.appendSecret(probeMarker)
	return a
}

const childProbe = `sh -c 'sh -c "sudo apt-get install -y x"' && echo children-ok`

var sudoGlobalOn = bgPolicy{sudoTimestampGlobal: true}

func TestSudoGlobalInstallsTheDropInAndStaysInTheBackgroundLane(t *testing.T) {
	for _, sh := range []string{"/bin/bash", "dash", "zsh"} {
		t.Run(sh, func(t *testing.T) {
			path, err := exec.LookPath(sh)
			if err != nil {
				t.Skipf("%s not installed", sh)
			}
			w := newSudoWorld(t)
			out, err := w.run(t, path, probeMarker, childProbe, secretAnswers(), sudoGlobalOn)
			if err != nil || !strings.Contains(out, "children-ok") {
				t.Fatalf("install.sh's children must sudo and the run must go on: err=%v\n%s", err, out)
			}
			want := "Defaults:" + currentUser(t) + " timestamp_type=global\n"
			if got := w.dropIn(); got != want {
				t.Fatalf("drop-in must be exactly %q, got %q", want, got)
			}
			if !strings.Contains(out, "installed /etc/sudoers.d/fleet-timestamp") {
				t.Fatalf("the run must SAY it changed the host:\n%s", out)
			}
			if strings.Contains(out, "parsed OK") {
				t.Fatalf("visudo's success chatter must not reach the stream:\n%s", out)
			}
			if left, _ := os.ReadDir(w.tmp); len(left) != 0 {
				t.Fatalf("root's temp sudoers file must be removed, left %d entries", len(left))
			}
			if strings.Contains(out, probeMarker) {
				t.Fatalf("the password leaked into the output:\n%s", out)
			}
		})
	}
}

func TestSudoGlobalOffKeepsTodaysReroute(t *testing.T) {
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, secretAnswers(), bgPolicy{})
	if shExit(err) != rcSudoNoCache {
		t.Fatalf("flag off: the gate must still exit %d so the host is rerouted, got err=%v out=%q", rcSudoNoCache, err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("flag off: the host must not be changed")
	}
}

func TestSudoGlobalNeverWritesWhenTheCredentialAlreadyReachesChildren(t *testing.T) {
	w := newSudoWorld(t)
	// A host already configured (or NOPASSWD): the gate passes first time.
	if err := os.WriteFile(w.sudoersD+"/site", []byte("Defaults timestamp_type=global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, secretAnswers(), sudoGlobalOn)
	if err != nil || !strings.Contains(out, "children-ok") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if w.dropIn() != "" || strings.Contains(out, "installed") {
		t.Fatalf("nothing to fix, so nothing may be written or announced:\n%s", out)
	}
}

func TestSudoGlobalRejectedPasswordWritesNothing(t *testing.T) {
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", "wrong", "echo must-not-run", secretAnswers(), sudoGlobalOn)
	if shExit(err) != rcSudoAuth || strings.Contains(out, "must-not-run") {
		t.Fatalf("a rejected password must exit %d before anything else: err=%v out=%q", rcSudoAuth, err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("a rejected password must not touch sudoers")
	}
}

// visudo refusing the drop-in (an old sudo without timestamp_type) must
// leave the host untouched AND say why on the stream — an opted-in operator
// cannot otherwise tell "refused" from "never ran".
func TestSudoGlobalVisudoRejectionFallsBackToTheRerouteAndSaysWhy(t *testing.T) {
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, secretAnswers(), sudoGlobalOn, "FAKE_VISUDO_FAIL=1")
	if shExit(err) != rcSudoNoCache {
		t.Fatalf("when visudo refuses the drop-in nothing is installed and the gate reroutes (92): err=%v out=%q", err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("an invalid drop-in must never be installed")
	}
	if !strings.Contains(out, "unknown defaults entry") {
		t.Fatalf("visudo's refusal must reach the stream:\n%s", out)
	}
}

// A host whose /etc/sudoers never includes sudoers.d: the drop-in installs
// cleanly and changes nothing. The run must not leave a root-owned file it
// announced as a fix — it removes it again, says so, and exits its own code
// so the model can name the real cause instead of "timestamp_type=tty".
func TestSudoGlobalInertDropInIsRemovedAgain(t *testing.T) {
	w := newSudoWorld(t)
	out, err := w.run(t, "/bin/bash", probeMarker, childProbe, secretAnswers(), sudoGlobalOn, "FAKE_INERT=1")
	if shExit(err) != rcSudoFixupInert || strings.Contains(out, "children-ok") {
		t.Fatalf("an inert drop-in must exit %d before the script runs: err=%v out=%q", rcSudoFixupInert, err, out)
	}
	if w.dropIn() != "" {
		t.Fatal("an inert drop-in must not be left behind")
	}
	if !strings.Contains(out, "installed /etc/sudoers.d/fleet-timestamp") || !strings.Contains(out, "removed it again") {
		t.Fatalf("the run must announce both the install and its retraction:\n%s", out)
	}
	// And the model names the cause on the status line.
	rep := updexec.HostReport{Results: []updexec.Result{{Step: "run1", Status: updexec.Failed, Exit: rcSudoFixupInert, Reason: "exit status 93"}}}
	derr := bgDoneErr(rep)
	if !sudoGateFailed(derr) || !errors.Is(derr, errSudoFixupInert) {
		t.Fatalf("exit %d must route like the gate AND carry its own sentinel, got %v", rcSudoFixupInert, derr)
	}
	m := testModel("h")
	m.updating["h"] = updState{phase: updRunning}
	m.running = 1
	got, _ := m.Update(bgUpdateDoneMsg{alias: "h", err: derr})
	if s := got.(tuiModel).status; !strings.Contains(s, sudoersDropIn) || !strings.Contains(s, "terminal lane") || strings.Contains(s, "timestamp_type=tty") {
		t.Fatalf("the status must name the drop-in and the reroute, not tty keying: %q", s)
	}
}

func TestSudoGlobalPreambleShape(t *testing.T) {
	a := secretAnswers()
	// Flag off is byte-for-byte the preamble every host ran before the flag
	// existed — the default lane must not drift while the fixup evolves.
	if off, want := bgPreamble(a, bgPolicy{})(updplan.Step{Kind: updplan.KindRun}),
		`sudo -S -p '' -v 2>/dev/null || exit 91; `+sudoGate+` || exit 92; `; off != want {
		t.Fatalf("flag off must be exactly today's preamble:\n got %s\nwant %s", off, want)
	}
	on := bgPreamble(a, sudoGlobalOn)(updplan.Step{Kind: updplan.KindRun})
	if strings.Contains(on, probeMarker) {
		t.Fatalf("the password leaked into the remote command:\n%s", on)
	}
	// Write, vet and install happen inside ONE root shell from a root-owned
	// temp file: nothing running as the operator can swap the vetted content
	// before install copies it.
	if !strings.Contains(on, `sudo -n sh -c 'f=$(mktemp) && cat >"$f" && visudo -cf "$f"`) {
		t.Fatalf("the drop-in must be written, vetted and installed by root from root's own temp file:\n%s", on)
	}
	// No credential: nothing to prime with, so nothing to fix — no sudoers logic.
	none := bgPreamble(answers{}, sudoGlobalOn)(updplan.Step{Kind: updplan.KindRun})
	if strings.Contains(none, "sudoers") || strings.Contains(none, "sudo -S") {
		t.Fatalf("no credential means no fixup:\n%s", none)
	}
}

// The lane policy is derived from the Settings the plan load already
// resolved, so featflag's fail-closed rule (TestResolveSudoTimestampGlobalIsFailClosed)
// is the only place that decides it; here we only pin the wiring.
func TestBgPolicyComesFromTheGffFlag(t *testing.T) {
	on := policyFrom(featflag.Resolve(featflag.Static{Bools: map[string]bool{featflag.KeySudoGlobal: true}}, "", t.TempDir()))
	if !on.sudoTimestampGlobal {
		t.Fatal("an explicit true must enable the lane fixup")
	}
	if policyFrom(featflag.Resolve(nil, "", "")).sudoTimestampGlobal {
		t.Fatal("no gff at all must stay off")
	}
	// resolveTUIPlan hands back the policy from its ONE resolution, and --file
	// still consults gff for it (the plan alone skips gff).
	dir := t.TempDir()
	file := dir + "/fleet.yaml"
	if err := os.WriteFile(file, []byte(updplan.DefaultYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	_, p, err := resolveTUIPlan(file, "", dir, featflag.Static{Bools: map[string]bool{featflag.KeySudoGlobal: true}})
	if err != nil || !p.sudoTimestampGlobal {
		t.Fatalf("resolveTUIPlan must return the policy from the same resolution as the plan: err=%v policy=%+v", err, p)
	}
}

func currentUser(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return u.Username
}

// The default was chosen on purpose (see the flag's description): the fixup
// fires only once the operator has typed the sudo password for that host, is
// announced in the stream, and one file undoes it — so it is ON, while the
// resolver stays fail-closed (TestBgPolicyComesFromTheGffFlag).
func TestSudoGlobalFlagDefaultsOn(t *testing.T) {
	b, err := os.ReadFile("../../../.github/gff/features.yaml")
	if err != nil {
		t.Skip("features.yaml not reachable from this checkout layout")
	}
	i := strings.Index(string(b), "path: "+featflag.KeySudoGlobal)
	if i < 0 {
		t.Fatalf("%s is not declared in features.yaml", featflag.KeySudoGlobal)
	}
	block := string(b)[i:]
	if j := strings.Index(block, "\n      - path:"); j > 0 {
		block = block[:j]
	}
	if !strings.Contains(block, "boolDefault: true") {
		t.Fatalf("%s must default to true (deliberate; see its description):\n%s", featflag.KeySudoGlobal, block)
	}
}
