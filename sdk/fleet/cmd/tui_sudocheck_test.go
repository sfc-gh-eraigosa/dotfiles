package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

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

// awaiting is a model with a check in flight, and the verdict that answers
// it — the state applySudoCheck acts on. A verdict carrying any other
// sequence number is one the model must ignore.
func awaiting(m tuiModel) (tuiModel, sudoCheckMsg) {
	m.sudoChecking = true
	m.sudoCheckSeq = 7
	return m, sudoCheckMsg{host: "h1", seq: 7}
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
	m, verdict := awaiting(sudoModel(t, runner.Fake{}))
	verdict.ok = false
	mm, _ := m.Update(verdict)
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
	// ...and the operator can SEE that: the form replaces the status bar,
	// so the verdict has to be drawn inside it.
	if view := m2.answersView(); !strings.Contains(view, "rejected") {
		t.Errorf("the form does not show the verdict:\n%s", view)
	}
}

func TestTheFormShowsTheCheckInProgress(t *testing.T) {
	m, _ := commit(sudoModel(t, runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=0"}}))
	if view := m.answersView(); !strings.Contains(view, "checking") {
		t.Errorf("the form gives no sign a check is running:\n%s", view)
	}
}

func TestASecondEnterDuringTheCheckIssuesNoSecondCheck(t *testing.T) {
	// Enter with no visible reaction invites another enter. That must not
	// become a second `sudo -S -v` against the host — the one in flight
	// decides.
	m, first := commit(sudoModel(t, runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=0"}}))
	if first == nil {
		t.Fatal("no check issued")
	}
	m2, second := commit(m)
	if second != nil {
		t.Fatal("a second enter fired a second check")
	}
	if m2.mode != modeAnswers {
		t.Fatalf("mode = %v, want the form to stay put until the verdict", m2.mode)
	}
}

func TestAStaleVerdictIsIgnored(t *testing.T) {
	// The operator typed, committed, then edited the password (or backed
	// out) before the host answered. That answer is about a password that
	// no longer exists: it must neither prove the new one nor wipe it.
	m, _ := commit(sudoModel(t, runner.Fake{Out: map[string]string{"h1": "fleet-sudo-rc=0"}}))
	m.ansField = fieldSudo
	edited, _ := send(m, "backspace") // the in-flight check is now about a different secret
	stale := sudoCheckMsg{host: "h1", ok: true, seq: m.sudoCheckSeq}
	mm, _ := edited.Update(stale)
	m2 := mm.(tuiModel)
	if m2.mode != modeAnswers || m2.sudoChecked {
		t.Fatalf("a verdict for an edited password was applied: mode=%v checked=%v", m2.mode, m2.sudoChecked)
	}

	// A rejection arriving after esc must not drag the form back open.
	cancelled, _ := send(m, "esc")
	mm, _ = cancelled.Update(sudoCheckMsg{host: "h1", ok: false, seq: m.sudoCheckSeq})
	m3 := mm.(tuiModel)
	if m3.mode != modeNormal {
		t.Fatalf("a late verdict reopened the form: mode=%v", m3.mode)
	}
	if m3.ans.secretLen() == 0 {
		t.Fatal("a late rejection wiped a password the operator kept")
	}
}

func TestTheCheckIsBoundedByADeadline(t *testing.T) {
	// A host whose sudo stalls in PAM must not hold the form open for good:
	// the check runs under a deadline and a timeout reads as uncheckable.
	r := runner.Fake{Block: map[string]bool{"h1": true}}
	// The Fake honours ctx, so drive the real command under a short deadline
	// and assert it reports rather than hangs.
	prev := sudoCheckTimeout
	sudoCheckTimeout = 200 * time.Millisecond
	t.Cleanup(func() { sudoCheckTimeout = prev })
	cmd := checkSudo(r, "h1", "x", 1)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if v, ok := msg.(sudoCheckMsg); !ok || !v.unreachable {
			t.Fatalf("a timed-out check must read as uncheckable, got %+v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the check did not return within its deadline")
	}
}

func TestAnAcceptedPasswordOpensTheConfirmStrip(t *testing.T) {
	m, verdict := awaiting(sudoModel(t, runner.Fake{}))
	verdict.ok = true
	mm, _ := m.Update(verdict)
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
	m, verdict := awaiting(sudoModel(t, runner.Fake{}))
	verdict.unreachable = true
	mm, _ := m.Update(verdict)
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

func TestTheTUIRefusesAPlanWhoseInputsOnlyAnOperatorCanAnswer(t *testing.T) {
	// The TUI has no form for plan inputs. A value the host can find for
	// itself is applied; one only the operator can supply has nowhere to
	// come from, so the update is refused before a host is contacted —
	// never started with the value silently unset.
	plan, err := updplan.Parse([]byte("version: 1\nupdate:\n  inputs:\n    - {id: node, scope: host, env: CONVERGE_NODE}\n  repos:\n    r: {path: ~/r}\n  steps:\n    - {id: s, kind: run, repo: r, run: ./x.sh}\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := newTUIModel([]sshconf.Host{{Alias: "h1"}}, runner.Fake{}, fakeBaseline{}, testNow, "main", 2, plan)
	if cmd := m.startUpdate([]string{"h1"}); cmd != nil {
		t.Fatal("an update started for a plan with an unanswered input")
	}
	if !strings.Contains(m.status, "h1:node") || !strings.Contains(m.status, "--input") {
		t.Errorf("status does not name the missing value and the way to supply it: %q", m.status)
	}
	if _, running := m.updating["h1"]; running {
		t.Error("the host was marked updating")
	}
}

func TestTheTUILaneAppliesThePlanInputsAHostCanAnswer(t *testing.T) {
	// The same plan runs through `fleet update` and the TUI; a default or a
	// host-side discovery must reach the step from both, after the sudo
	// preamble, or the two lanes converge different things.
	plan, err := updplan.Parse([]byte("version: 1\nupdate:\n  inputs:\n    - {id: lane, default: fast}\n    - {id: node, scope: host, env: CONVERGE_NODE, default_from: \"hostname -s\"}\n  repos:\n    r: {path: ~/r}\n  steps:\n    - {id: s, kind: run, repo: r, run: ./x.sh}\n"))
	if err != nil {
		t.Fatal(err)
	}
	var a answers
	a.appendSecret("hunter2")
	preamble, stdin := bgLaneIO("h1", plan, a, bgPolicy{})
	st, _ := plan.Step("s")
	pre := preamble(st)
	sudoAt, laneAt := strings.Index(pre, sudoPrime), strings.Index(pre, "export LANE='fast'")
	if sudoAt < 0 || laneAt < 0 || laneAt < sudoAt {
		t.Errorf("the plan default does not follow the sudo preamble: %q", pre)
	}
	if !strings.Contains(pre, "hostname -s") || !strings.Contains(pre, "export CONVERGE_NODE") {
		t.Errorf("host-side discovery is missing from the TUI lane: %q", pre)
	}
	if got := stdin(st); got != "hunter2\n" {
		t.Errorf("stdin = %q, want the sudo line alone when no confidential input travels", got)
	}
}
