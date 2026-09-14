package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// These pin the three defects found on the first real fleet run. Each one
// caused install.sh to stop and ask a question the operator had already
// answered — the exact failure the unattended path exists to prevent.

// DEFECT 1: envPrefix used the `VAR=x cmd` form, which scopes the assignment
// to that ONE command. `WINSETUP_ANSWER=s cd ~/git/dotfiles && ./install.sh`
// set it for `cd` and nothing else, so install.sh never saw it.
// Verified in a shell: `sh -c 'FOO=bar cd /tmp && env | grep FOO'` prints
// nothing.
func TestAnswersAreExportedSoInstallShInheritsThem(t *testing.T) {
	a := answers{windows: "s", gemini: "keep"}
	script := bgPreamble(a)(updplan.Step{Kind: updplan.KindRun}) + "cd ~/git/dotfiles && ./install.sh"
	if !strings.Contains(script, "export WINSETUP_ANSWER=s") {
		t.Fatalf("answers must be exported, not prefixed:\n%s", script)
	}
	// The prefix form is the exact shape that silently failed.
	if strings.Contains(script, "WINSETUP_ANSWER=s cd ") {
		t.Fatalf("regression: prefix form never reaches install.sh:\n%s", script)
	}
	if i, j := strings.Index(script, "export "), strings.Index(script, "cd ~"); i < 0 || i > j {
		t.Fatalf("the export must precede the command chain:\n%s", script)
	}
}

// DEFECT 2: a host whose sudo needs a password was routed to the interactive
// lane EVEN WHEN the operator had supplied one — so they were prompted for the
// password they had just typed. `sudo -n` failing means "needs a password",
// not "must be interactive": with one in hand the background lane primes it.
func TestSuppliedCredentialKeepsAPasswordHostInTheBackgroundLane(t *testing.T) {
	m := newTUIModel(hosts("needs-pw"), nil, fakeBaseline{head: "abc"}, testNow, "main", 2, updplan.Default())
	m.ans = answers{sudoSecret: strings.Repeat("Zq7", 3)}
	mm, _ := m.Update(precheckMsg{alias: "needs-pw", interactive: true})
	got := mm.(tuiModel)
	if got.iaTotal != 0 {
		t.Fatalf("with a credential supplied the host must NOT go interactive (iaTotal=%d)", got.iaTotal)
	}
	if got.updating["needs-pw"].phase != updRunning {
		t.Fatalf("it should have started in the background, phase=%v", got.updating["needs-pw"].phase)
	}

	// Without one it must still fall back, or a password-needing host would
	// fail in the background with nobody able to answer.
	m2 := newTUIModel(hosts("needs-pw"), nil, fakeBaseline{head: "abc"}, testNow, "main", 2, updplan.Default())
	mm2, _ := m2.Update(precheckMsg{alias: "needs-pw", interactive: true})
	if mm2.(tuiModel).iaTotal != 1 {
		t.Fatal("with no credential the host must fall back to the interactive lane")
	}
}

// DEFECT 3: the interactive lane used to run a bare remote script embedding
// the answers as an "export" prefix on the remote command; now the child is
// a self-exec of `fleet update`, and the answers travel as its OWN
// environment (never the remote shell's) — see handoffEnv.
func TestInteractiveLaneCarriesTheAnswers(t *testing.T) {
	a := answers{windows: "s", gemini: "keep"}
	joined := strings.Join(handoffEnv(a), "\n")
	for _, want := range []string{"WINSETUP_ANSWER=s", "GEMINI_TEARDOWN_ANSWER=keep"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("interactive handoff dropped %q from its environment", want)
		}
	}
}

// While a handoff runs the TUI is suspended, so the screen is bare install.sh
// output. Without a banner there is nothing saying which machine it belongs to.
func TestHandoffAnnouncesHostAndProgress(t *testing.T) {
	w := handoffWrapper("host-b", "main", "echo remote", 2, 3)
	for _, want := range []string{"host-b", "host 2 of 3", "finished"} {
		if !strings.Contains(w, want) {
			t.Fatalf("handoff wrapper missing %q:\n%s", want, w)
		}
	}
	// The remote's exit code must survive the wrapper, or every handoff would
	// look successful.
	if !strings.Contains(w, "exit $rc") {
		t.Fatalf("wrapper must propagate the remote exit code:\n%s", w)
	}
}

// git prints advice AFTER the real error, so a naive tail showed only the
// advice. Observed live: `FAIL: exit status 128 hint: | hint: Disable this
// message with "git config advice…`.
func TestGitAdviceIsStrippedSoTheRealErrorSurvives(t *testing.T) {
	out := "error: pathspec 'main' did not match any file(s)\n" +
		"hint: Disable this message with \"git config advice.x false\"\nhint: | \n"
	got := tailLines(out, 3)
	if strings.Contains(strings.ToLower(got), "hint:") {
		t.Fatalf("git advice must not crowd out the error: %q", got)
	}
	if !strings.Contains(got, "pathspec") {
		t.Fatalf("the real error must survive: %q", got)
	}
}

// The form is a dialog, not loose text floating under the table.
func TestAnswerFormIsFramedAsADialog(t *testing.T) {
	m := newTUIModel(hosts("a"), nil, fakeBaseline{head: "abc"}, testNow, "main", 2, updplan.Default())
	m.mode = modeAnswers
	view := m.View()
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
		t.Fatalf("the answer form must be framed:\n%s", view)
	}
	if !strings.Contains(view, "esc: cancel") {
		t.Fatalf("the dialog must document its keys:\n%s", view)
	}
}

// install.sh treats its own failed `sudo -v` as non-fatal and keeps going, so
// starting it without usable sudo produced a cascade of "sudo: a password is
// required" and could still exit 0 — leaving the row reading `ok` while every
// privileged step had silently skipped. The gate must therefore be
// UNCONDITIONAL, not merely present when a credential was supplied.
func TestSudoIsGatedEvenWithNoCredential(t *testing.T) {
	installStep := updplan.Step{Kind: updplan.KindRun}
	noCred := bgPreamble(answers{})(installStep) + "cd ~/git/dotfiles && ./install.sh"
	if !strings.Contains(noCred, "sudo -n true") {
		t.Fatalf("a credential-less run must still verify sudo before installing:\n%s", noCred)
	}
	if i, j := strings.Index(noCred, "sudo -n true"), strings.Index(noCred, "install.sh"); i < 0 || i > j {
		t.Fatalf("the gate must precede install.sh:\n%s", noCred)
	}
	// And it still primes when there IS one.
	withSecret := answers{}
	withSecret.appendSecret(probeMarker)
	withCred := bgPreamble(withSecret)(installStep) + "cd ~/git/dotfiles && ./install.sh"
	if !strings.Contains(withCred, "sudo -S -p '' -v") {
		t.Fatalf("a supplied credential must still be primed:\n%s", withCred)
	}
}

// Hosts that legitimately need no sudo must be exempt, not blocked.
func TestSudoGateExemptsRootAndSudolessHosts(t *testing.T) {
	if !strings.Contains(sudoGate, `[ "$(id -u)" = 0 ]`) {
		t.Fatalf("root must be exempt from the gate: %s", sudoGate)
	}
	if !strings.Contains(sudoGate, "command -v sudo") {
		t.Fatalf("a host without sudo must be exempt: %s", sudoGate)
	}
	// Executed for real: on a PATH with no `sudo`, the gate must pass.
	c := exec.Command("/bin/bash", "-c", sudoGate+" && echo exempt")
	c.Env = []string{"PATH=/nonexistent-for-test"} // a host with no sudo at all
	out, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "exempt") {
		t.Fatalf("gate blocked a sudoless host: err=%v out=%q", err, out)
	}
}

// The failure must name the cause; "exit status 92" alone is useless, and the
// operator needs to know nothing was installed.
func TestUnusableSudoIsExplainedAsNothingInstalled(t *testing.T) {
	got := explainExit(&fakeErr{"exit status 92"})
	for _, want := range []string{"sudo", "nothing was installed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("explainExit should mention %q, got %q", want, got)
		}
	}
}

// DEFECT 4 (root-caused from `fleet history <host> --show --run 1` on a host
// without passwordless sudo): sudo's default timestamp_type=tty has no tty
// to key on when the session was primed over ssh, so it falls back to the
// PPID of whatever process ran `sudo`. The old gate ran `sudo -n true`
// directly in the SAME shell that had just primed the credential — same
// PPID, so it always passed — while install.sh's own children
// (opt/bin/pkg-install-apt, the keep-alive loop, the docker step) each have a
// DIFFERENT PPID (install.sh's, not the top shell's) and never saw the
// credential. The gate must instead test from a CHILD process, exactly the
// shape install.sh's children are in, or it proves nothing.
//
// This is exercised for real: a fake `sudo` on PATH records which PPID
// primed it and only answers `-n true` for that same PPID — reproducing the
// live host's behaviour without touching a real sudoers file.
func TestSudoGateChecksFromAChildProcess(t *testing.T) {
	if !strings.Contains(sudoGate, "sh -c") {
		t.Fatalf("the gate must fork a child before testing sudo, or it shares the primer's PPID and always passes:\n%s", sudoGate)
	}

	dir := t.TempDir()
	fakeSudo := dir + "/sudo"
	// Records the caller's PPID on a bare prime (`-v`); a `-n true` check
	// succeeds only when invoked with that SAME PPID — modelling
	// timestamp_type=tty falling back to PPID scoping with no tty.
	script := `#!/bin/sh
if [ "$1" = "-n" ]; then
  [ -f "` + dir + `/primed.$PPID" ]
  exit $?
fi
touch "` + dir + `/primed.$PPID"
`
	if err := os.WriteFile(fakeSudo, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir + ":/usr/bin:/bin"}

	// Sanity: priming and a BARE `sudo -n true` in the SAME shell (the
	// pre-fix gate's shape) share one PPID and trivially pass — this is
	// exactly the false "cached" reading the live host produced.
	same := exec.Command("/bin/bash", "-c", "sudo -v; sudo -n true && echo ok")
	same.Env = env
	if out, err := same.CombinedOutput(); err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("sanity check failed: priming and a bare check in one shell should share a PPID: err=%v out=%q", err, out)
	}

	// The actual gate forks a child before checking (see the "sh -c" assertion
	// above) — the shape install.sh's own children are in. In that SAME
	// primed session, the credential must NOT be visible there.
	forked := exec.Command("/bin/bash", "-c", "sudo -v; "+sudoGate+" && echo ok")
	forked.Env = env
	out, err := forked.CombinedOutput()
	if err == nil || strings.Contains(string(out), "ok") {
		t.Fatalf("gate must fail from a child process when the credential does not carry to children: err=%v out=%q", err, out)
	}

	// Where /bin/sh is bash (macOS, Fedora, Arch), `sh -c '<one command>'`
	// execs that command in place instead of forking, handing sudo the
	// primer's PPID again. The gate must still fork there.
	bashPath, lerr := exec.LookPath("bash")
	if lerr != nil {
		t.Skip("no bash on PATH")
	}
	shDir := t.TempDir()
	if err := os.Symlink(bashPath, shDir+"/sh"); err != nil {
		t.Fatal(err)
	}
	bashSh := exec.Command("/bin/bash", "-c", "sudo -v; "+sudoGate+" && echo ok")
	bashSh.Env = []string{"PATH=" + dir + ":" + shDir + ":/usr/bin:/bin"}
	out, err = bashSh.CombinedOutput()
	if err == nil || strings.Contains(string(out), "ok") {
		t.Fatalf("gate must still fork when /bin/sh is bash (single-command -c exec optimization): err=%v out=%q", err, out)
	}
}

// bgDoneErr is what lets tui_model route a host away from the background lane
// on exactly the sudoGate's rcSudoNoCache exit (92). It reads the step's TYPED
// exit code: matching "92" in the error text also matched exit 192 and any
// reason that merely mentioned a 192.168.x address, sending a genuinely failed
// host to the terminal lane.
func TestBgDoneErrRecognisesOnlyTheGateExitCode(t *testing.T) {
	report := func(exit int, reason string) updexec.HostReport {
		return updexec.HostReport{Results: []updexec.Result{
			{Step: "sync", Status: updexec.OK},
			{Step: "run1", Status: updexec.Failed, Exit: exit, Reason: reason},
		}}
	}
	if err := bgDoneErr(report(rcSudoNoCache, "exit status 92")); !sudoGateFailed(err) {
		t.Fatalf("the gate's exit code must be recognised, got %v", err)
	}
	for _, tc := range []struct {
		exit   int
		reason string
	}{
		{rcSudoAuth, "exit status 91"},                            // a rejected password is not a gate failure
		{192, "exit status 192"},                                  // contains "92"
		{1, "ssh: connect to host 192.168.1.92 port 22: refused"}, // mentions "92"
	} {
		if err := bgDoneErr(report(tc.exit, tc.reason)); sudoGateFailed(err) {
			t.Fatalf("exit %d (%q) must not read as a gate failure", tc.exit, tc.reason)
		}
	}
	if err := bgDoneErr(updexec.HostReport{Results: []updexec.Result{{Step: "run1", Status: updexec.OK}}}); err != nil {
		t.Fatalf("a clean run is no error, got %v", err)
	}
	if sudoGateFailed(nil) {
		t.Fatal("a nil error is not a gate failure")
	}
	// The row still explains itself: the step's own reason survives the wrap.
	if err := bgDoneErr(report(rcSudoNoCache, "exit status 92")); !strings.Contains(err.Error(), "step run1: exit status 92") {
		t.Fatalf("the wrapped error must keep the step's reason, got %v", err)
	}
}
