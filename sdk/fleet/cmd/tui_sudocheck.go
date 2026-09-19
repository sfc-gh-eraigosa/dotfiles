package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// Verifying the sudo password before the wave, rather than discovering it is
// wrong once every host has already tried to prime with it.
//
// The old order made a typo expensive: the form accepted anything, the wave
// started, and each host failed its own prime (rc 91) one after another —
// a whole fleet's worth of failures for one mistyped character, with the
// password already forgotten by the time you read the errors.

// sudoVerifyRC is the marker the check echoes so the outcome is read from
// STDOUT rather than inferred from an ssh exit status, which conflates "the
// password was wrong" with "the host was not there".
const sudoVerifyRC = "fleet-sudo-rc="

// sudoVerify primes sudo exactly the way the wave will, and reports the
// result. The three benign cases — already root, no sudo installed, a
// credential that needs no password — all answer 0 (the same exemption
// sudoGate grants, spelled once in sudoExempt), so the check only ever
// fails on a password sudo actually rejected.
var sudoVerify = fmt.Sprintf(`{ %s || %s; }; echo %s$?`, sudoExempt, sudoPrime, sudoVerifyRC)

// sudoCheckTimeout bounds the whole round trip. The wave bounds every
// attempt with the step's timeout; the pre-wave question deserves the same
// courtesy, or a host whose PAM stack has stalled holds the form open with
// no way out. Well past any sane ssh+sudo, well short of "is it hung?".
var sudoCheckTimeout = 30 * time.Second

// sudoCheckMsg is the verdict. unreachable means the question could not be
// put to the host at all, which says nothing about the password. seq is
// the check it answers; the model drops a verdict for any other.
type sudoCheckMsg struct {
	host        string
	ok          bool
	unreachable bool
	detail      string
	seq         int
}

// checkSudo asks one host whether this credential actually authenticates.
// The secret travels on stdin, never argv, exactly as it does during a run.
// RunStreamCtx rather than RunStdin because it is the one Runner call with
// a deadline that kills the local ssh, not merely abandons it.
func checkSudo(r runner.Runner, host, secret string, seq int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), sudoCheckTimeout)
		defer cancel()
		lines, done := r.RunStreamCtx(ctx, host, secret+"\n", sudoVerify)
		var out strings.Builder
		for l := range lines {
			out.WriteString(l)
			out.WriteString("\n")
		}
		if err := <-done; err != nil {
			// Could not ask (or not in time). Not a verdict on the password.
			return sudoCheckMsg{host: host, unreachable: true, detail: err.Error(), seq: seq}
		}
		rc, found := parseSudoRC(out.String())
		if !found {
			return sudoCheckMsg{host: host, unreachable: true, detail: "no result marker in the reply", seq: seq}
		}
		return sudoCheckMsg{host: host, ok: rc == 0, seq: seq}
	}
}

// parseSudoRC pulls the exit status out of the reply.
func parseSudoRC(out string) (int, bool) {
	i := strings.LastIndex(out, sudoVerifyRC)
	if i < 0 {
		return 0, false
	}
	field := strings.TrimSpace(out[i+len(sudoVerifyRC):])
	if nl := strings.IndexAny(field, " \t\r\n"); nl >= 0 {
		field = field[:nl]
	}
	rc, err := strconv.Atoi(field)
	if err != nil {
		return 0, false
	}
	return rc, true
}

// dropSudoCheck forgets any verdict — given or still in flight — because
// the thing it was about (the secret, or the form itself) is gone.
func (m *tuiModel) dropSudoCheck() {
	m.sudoChecked = false
	m.sudoChecking = false
	m.sudoCheckSeq++
}

// applySudoCheck folds the verdict back into the model. A verdict for a
// check the model is no longer waiting on — the secret was edited, the form
// was cancelled, a newer check superseded it — is dropped unread: the form
// it would act on is not the form that asked.
func (m tuiModel) applySudoCheck(msg sudoCheckMsg) (tea.Model, tea.Cmd) {
	if !m.sudoChecking || msg.seq != m.sudoCheckSeq || m.mode != modeAnswers {
		return m, nil
	}
	m.sudoChecking = false
	switch {
	case msg.ok:
		m.sudoChecked = true
		m.mode = modeConfirm
		m.status = fmt.Sprintf("sudo password accepted by %s", msg.host)
	case msg.unreachable:
		// The host could not answer, so the password is neither proven nor
		// disproven. Blocking here would turn one unreachable machine into a
		// refusal to update the rest; the wave handles a bad credential per
		// host anyway.
		m.sudoChecked = true
		m.mode = modeConfirm
		m.status = fmt.Sprintf("could not check the sudo password on %s (%s) — continuing", msg.host, msg.detail)
	default:
		// Back to the field, cleared: a rejected password is almost always a
		// typo, and retyping over the old one is how the second attempt goes
		// wrong too.
		m.ans.clearSecret()
		m.sudoChecked = false
		m.mode = modeAnswers
		m.ansField = fieldSudo
		m.status = fmt.Sprintf("sudo password rejected by %s — type it again", msg.host)
	}
	return m, nil
}
