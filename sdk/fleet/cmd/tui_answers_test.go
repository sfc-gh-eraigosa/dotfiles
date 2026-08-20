package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// probeMarker stands in for a real credential. The tests below assert it never
// escapes into argv or the environment, so it has to be distinctive/greppable.
var probeMarker = strings.Repeat("Zq7", 3)

// The credential must never be reachable through argv or the environment:
// /proc/<pid>/cmdline and /proc/<pid>/environ are world-readable, so anything
// placed in either leaks to every user on the host.
func TestSudoSecretNeverAppearsInTheRemoteCommand(t *testing.T) {
	a := answers{sudoSecret: probeMarker, windows: "s", gemini: "keep"}
	script := unattendedUpdate("main", a)
	if strings.Contains(script, probeMarker) {
		t.Fatalf("the credential leaked into the remote command:\n%s", script)
	}
	// The non-secret answers DO travel as environment — that is their contract.
	for _, want := range []string{"WINSETUP_ANSWER=s", "GEMINI_TEARDOWN_ANSWER=keep"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q in:\n%s", want, script)
		}
	}
}

// It must arrive over stdin instead — the only channel not visible to other
// processes on either end.
func TestSudoSecretIsSentOnStdinOnly(t *testing.T) {
	seen := map[string]string{}
	r := runner.Fake{Stdin: seen}
	msg := beginStream("host-a", "main", answers{sudoSecret: probeMarker}, r)()
	st, ok := msg.(streamStartedMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	<-st.st.done // drain so the fake has recorded stdin
	if got := seen["host-a"]; !strings.HasPrefix(got, probeMarker) {
		t.Fatalf("credential did not reach stdin, got %q", got)
	}
}

// With nothing supplied we must not emit a sudo preamble at all — an empty
// `sudo -S` would consume nothing and fail confusingly.
func TestNoSecretMeansNoSudoPreamble(t *testing.T) {
	script := unattendedUpdate("main", answers{windows: "n"})
	if strings.Contains(script, "sudo -S") {
		t.Fatalf("nothing supplied, so no sudo preamble belongs here:\n%s", script)
	}
	if !strings.Contains(script, "WINSETUP_ANSWER=n") {
		t.Fatal("prompt answers must still apply without a credential")
	}
}

// Priming and installing must share ONE ssh session: sudo's timestamp is
// tty/session-scoped, so a prime in a separate connection may not be visible.
func TestPrimeAndInstallShareOneSession(t *testing.T) {
	script := unattendedUpdate("main", answers{sudoSecret: probeMarker})
	primeAt := strings.Index(script, "sudo -S")
	installAt := strings.Index(script, "install.sh")
	if primeAt < 0 || installAt < 0 || primeAt > installAt {
		t.Fatalf("prime must precede install in the same script:\n%s", script)
	}
	// And it must be VERIFIED, not assumed — otherwise a long install runs
	// with every privileged step silently skipping.
	if !strings.Contains(script, "sudo -n true") {
		t.Fatalf("the primed credential must be verified before installing:\n%s", script)
	}
}

// A sudo problem must be distinguishable from a genuine install failure.
func TestSudoFailuresAreExplained(t *testing.T) {
	for errText, want := range map[string]string{
		"exit status 91": "authentication failed",
		"exit status 92": "did not persist",
	} {
		if got := explainExit(&fakeErr{errText}); !strings.Contains(got, want) {
			t.Errorf("explainExit(%q) = %q, want it to mention %q", errText, got, want)
		}
	}
}

type fakeErr struct{ s string }

func (e *fakeErr) Error() string { return e.s }

// --- the form ------------------------------------------------------------

// `u` opens the answer form first (operator chose "ask me per wave"), and it
// must start empty so a wave never inherits a previous wave's answers.
// remembered builds an answer set as if a previous wave had filled it in,
// without spelling a credential literal into a struct field.
func remembered() answers {
	var a answers
	a.appendSecret(probeMarker)
	a.windows, a.gemini = "y", "keep"
	return a
}

// F19b — the FIRST wave of a session has nothing remembered, so `u` must open
// the form.
//
// This replaces TestUpdateOpensAnEmptyAnswerForm, which asserted the form was
// emptied on EVERY `u`. That rule existed to stop a wave inheriting stale
// answers; in practice it forced the operator to retype the password for every
// wave of a fleet-wide update, and retyping is precisely how two waves end up
// applying different answers. The protection now comes from the confirm strip
// showing what will be applied (F20a) rather than from forgetting it.
func TestFirstWaveOpensTheAnswerForm(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "u")
	if m2.mode != modeAnswers {
		t.Fatalf("u must open the answer form when nothing is remembered, mode=%v", m2.mode)
	}
	if m2.ans.secretLen() != 0 || m2.ans.windows != "" {
		t.Fatalf("the first form of a session must start empty, got %+v", m2.ans)
	}
}

// F19c — a later wave skips the form: the answers are already known, and the
// confirm strip is where they get one last look.
func TestLaterWaveSkipsStraightToConfirm(t *testing.T) {
	m := testModel("a", "b")
	m.ans = remembered()

	m2, _ := send(m, "u")
	if m2.mode != modeConfirm {
		t.Fatalf("u with remembered answers must go straight to confirm, mode=%v", m2.mode)
	}
	if m2.ans.secretLen() == 0 || m2.ans.windows != "y" {
		t.Fatalf("remembered answers must survive into the wave, got %+v", m2.ans)
	}
}

// F19d — the whole point: change the targets, keep the answers. Re-typing a
// password because the selection changed is what made fleet-wide updates
// inconsistent in the first place.
func TestAnswersSurviveASelectionChange(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "u")       // form
	m3, _ := send(m2, "p", "w") // type a credential
	m4, _ := send(m3, "esc")    // back out
	m5, _ := send(m4, "j", " ") // move and select a different host

	if m5.ans.secretLen() != 2 {
		t.Fatalf("changing the selection must not forget the answers, got %+v", m5.ans)
	}
	m6, _ := send(m5, "u")
	if m6.mode != modeConfirm {
		t.Fatalf("the next wave should reuse them, mode=%v", m6.mode)
	}
}

// Typed characters are text, never actions — 'q' must not quit, 'j' must not move.
func TestAnswerFormSwallowsActionKeys(t *testing.T) {
	m := testModel("a", "b")
	m2, _ := send(m, "u")
	start := m2.cursor
	m3, cmd := send(m2, "q", "j", "y")
	if cmd != nil {
		t.Fatal("typing in the credential field must not trigger an action (quit)")
	}
	if m3.cursor != start {
		t.Fatal("typing must not move the cursor")
	}
	if m3.ans.secretLen() != 3 {
		t.Fatalf("expected 3 typed characters, got %d", m3.ans.secretLen())
	}
}

// It must never be rendered — only a mask of its length.
func TestAnswerFormMasksTheSecret(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "u")
	m3, _ := send(m2, "s", "e", "c", "r", "e", "t")
	view := m3.View()
	if strings.Contains(view, "secret") {
		t.Fatalf("the typed credential must never be rendered:\n%s", view)
	}
	if !strings.Contains(view, strings.Repeat("•", 6)) {
		t.Fatalf("expected a 6-character mask:\n%s", view)
	}
}

// F19a — esc backs OUT of the form; it no longer forgets.
//
// This replaces TestEscapingTheFormDiscardsTheSecret. Its deletion is the
// deliberate reversal of the v2 invariant, not an oversight: forgetting on esc
// meant an operator who glanced away from the form paid for it with a retype.
// Forgetting is now an explicit act (`F`, see TestForgetClearsEverything) and
// still unconditional at process exit.
func TestEscapingTheFormKeepsTheAnswers(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "u")
	m3, _ := send(m2, "p", "w")
	m4, _ := send(m3, "esc")

	if m4.ans.secretLen() != 2 {
		t.Fatal("esc must back out of the form without forgetting the answers")
	}
	if m4.mode != modeNormal {
		t.Fatal("esc must return to normal mode")
	}
}

func TestAnswerFormNavigatesAndSetsChoices(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "u")
	m3, _ := send(m2, "tab") // -> windows
	m4, _ := send(m3, "s")   // skip forever
	m5, _ := send(m4, "tab") // -> gemini
	m6, _ := send(m5, "k")   // keep
	if m6.ans.windows != "s" || m6.ans.gemini != "keep" {
		t.Fatalf("choices not recorded: %+v", m6.ans)
	}
	// enter from the last field opens the confirm strip, not the wave itself.
	m7, cmd := send(m6, "enter", "enter") // gemini -> reset -> confirm
	if m7.mode != modeConfirm || cmd != nil {
		t.Fatalf("last-field enter must open the confirm strip, mode=%v", m7.mode)
	}
}

// The answers must survive form -> confirm -> wave and reach the host.
func TestAnswersReachTheRemoteCommand(t *testing.T) {
	m := testModel("a")
	m2, _ := send(m, "u")
	m3, _ := send(m2, "p", "w")   // credential field
	m4, _ := send(m3, "tab", "s") // windows = s
	m5, _ := send(m4, "tab", "y") // gemini = yes
	m6, _ := send(m5, "enter")    // -> confirm
	m7, _ := send(m6, "y")        // -> wave
	if m7.ans.windows != "s" || m7.ans.gemini != "yes" || m7.ans.secretLen() != 2 {
		t.Fatalf("answers lost on the way to the wave: %+v", m7.ans)
	}
	script := unattendedUpdate(m7.updateRef, m7.ans)
	if !strings.Contains(script, "WINSETUP_ANSWER=s") || !strings.Contains(script, "GEMINI_TEARDOWN_ANSWER=yes") {
		t.Fatalf("answers did not reach the remote command:\n%s", script)
	}
}
