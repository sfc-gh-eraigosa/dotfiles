package cmd

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// A mistyped sudo password used to cost the whole wave: nothing checked it
// until each host tried to prime, by which time the run was under way. The
// form now verifies it against one host on commit, so a typo costs one retry.

func sudoModel(t *testing.T, r runner.Runner) tuiModel {
	t.Helper()
	m := newTUIModel(
		[]sshconf.Host{{Alias: "h1"}, {Alias: "h2"}},
		r, fakeBaseline{}, testNow, "main", 2, updplan.Default(),
	)
	m.mode = modeAnswers
	m.ansField = answerFieldCount - 1 // committing from the last field
	m.ans.appendSecret("hunter2")
	return m
}

func commit(m tuiModel) (tuiModel, tea.Cmd) {
	mm, cmd := routeAnswers(m, tea.KeyMsg{Type: tea.KeyEnter})
	return mm.(tuiModel), cmd
}

func TestSudoPasswordIsVerifiedBeforeTheConfirmStrip(t *testing.T) {
	m, cmd := commit(sudoModel(t, runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=0"}}))
	if cmd == nil {
		t.Fatal("committing with a password issued no check")
	}
	if m.mode == modeConfirm {
		t.Fatal("the confirm strip opened before the password was checked")
	}
	if !strings.Contains(strings.ToLower(m.status), "check") {
		t.Errorf("status does not say a check is running: %q", m.status)
	}

	// The check itself: the secret goes on STDIN, never in the command.
	msg := cmd()
	got, ok := msg.(sudoCheckMsg)
	if !ok {
		t.Fatalf("check produced %T, want sudoCheckMsg", msg)
	}
	if !got.ok {
		t.Fatalf("rc=0 should read as accepted: %+v", got)
	}
}

func TestTheSecretIsNotInTheVerifyCommand(t *testing.T) {
	seen := map[string]string{}
	r := runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=0"}, Stdin: seen}
	_, cmd := commit(sudoModel(t, r))
	cmd()
	if seen["h1"] != "hunter2\n" {
		t.Fatalf("the password did not reach stdin: %q", seen["h1"])
	}
	// sudoVerify is a constant; assert the credential is not baked into it.
	if strings.Contains(sudoVerify, "hunter2") {
		t.Fatal("the credential is in the verify command")
	}
}

func TestARejectedPasswordReturnsToTheFieldAndClearsIt(t *testing.T) {
	m := sudoModel(t, runner.Fake{})
	m.mode = modeAnswers
	mm, _ := m.Update(sudoCheckMsg{host: "h1", ok: false})
	m2 := mm.(tuiModel)

	if m2.mode != modeAnswers {
		t.Fatalf("mode = %v, want the form to stay open for another try", m2.mode)
	}
	if m2.ansField != fieldSudo {
		t.Errorf("cursor = %v, want it back on the password field", m2.ansField)
	}
	if m2.ans.secretLen() != 0 {
		t.Error("the rejected password was kept — retyping must start clean")
	}
	if !strings.Contains(m2.status, "h1") || !strings.Contains(strings.ToLower(m2.status), "reject") {
		t.Errorf("status does not say what happened: %q", m2.status)
	}
}

func TestAnAcceptedPasswordOpensTheConfirmStrip(t *testing.T) {
	m := sudoModel(t, runner.Fake{})
	mm, _ := m.Update(sudoCheckMsg{host: "h1", ok: true})
	m2 := mm.(tuiModel)
	if m2.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm", m2.mode)
	}
	if m2.ans.secretLen() == 0 {
		t.Error("an accepted password must be kept")
	}
}

func TestAHostThatCannotBeCheckedDoesNotBlockTheRun(t *testing.T) {
	// An unreachable host says nothing about the password. Refusing to run
	// would be worse than running: the wave has its own per-host handling.
	m := sudoModel(t, runner.Fake{})
	mm, _ := m.Update(sudoCheckMsg{host: "h1", ok: false, unreachable: true})
	m2 := mm.(tuiModel)
	if m2.mode != modeConfirm {
		t.Fatalf("mode = %v, want the run to proceed", m2.mode)
	}
	if !strings.Contains(strings.ToLower(m2.status), "could not") {
		t.Errorf("status does not admit the check was skipped: %q", m2.status)
	}
	if m2.ans.secretLen() == 0 {
		t.Error("an unverifiable password must not be discarded")
	}
}

func TestNoPasswordSkipsTheCheckEntirely(t *testing.T) {
	// An empty password is a deliberate answer ("skip privileged steps"),
	// not something to verify.
	m := sudoModel(t, runner.Fake{})
	m.ans = answers{}
	m.ansField = answerFieldCount - 1
	m2, cmd := commit(m)
	if m2.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm with no password to check", m2.mode)
	}
	if cmd != nil {
		// saveAnswers may return a cmd; what matters is no check ran.
		if msg := cmd(); msg != nil {
			if _, isCheck := msg.(sudoCheckMsg); isCheck {
				t.Fatal("an empty password was sent for verification")
			}
		}
	}
}

func TestATransportErrorIsReportedAsUncheckable(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"h1": errors.New("ssh: connect: no route to host")}}
	_, cmd := commit(sudoModel(t, r))
	msg := cmd().(sudoCheckMsg)
	if !msg.unreachable {
		t.Fatalf("a transport failure must read as uncheckable, got %+v", msg)
	}
	if msg.ok {
		t.Fatal("a transport failure is not an accepted password")
	}
}

func TestARejectedRcIsReportedAsRejected(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=1"}}
	_, cmd := commit(sudoModel(t, r))
	msg := cmd().(sudoCheckMsg)
	if msg.ok || msg.unreachable {
		t.Fatalf("rc=1 must read as a rejected password, got %+v", msg)
	}
}
