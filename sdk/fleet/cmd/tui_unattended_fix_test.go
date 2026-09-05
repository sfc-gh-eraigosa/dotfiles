package cmd

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	"os/exec"
	"strings"
	"testing"
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
