package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// Messages carrying async results back into Update().
type (
	hostRowMsg  struct{ row Row }
	precheckMsg struct {
		alias       string
		interactive bool // sudo needs a password → must have a real terminal
	}
	bgUpdateDoneMsg struct {
		alias, log string
		err        error
	}
	execDoneMsg struct {
		alias string
		err   error
		ssh   bool // true = plain ssh visit, false = interactive update handoff
	}
	wakeDoneMsg struct {
		alias, via string
		woke       bool
		attempts   []reach.Attempt
	}
	spinnerTickMsg int
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinnerTick(n int) tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg(n) })
}

// pollHost probes one host off the UI thread and streams the row back (F1).
func pollHost(h sshconf.Host, r runner.Runner, base Baseliner) tea.Cmd {
	return pollHostWake(h, nil, r, base, nil)
}

// pollHostWake is pollHost with auto-wake. The TUI reaches the ladder through
// the SAME probe path as the headless command, so the two can never disagree
// about what `unreachable` means.
func pollHostWake(h sshconf.Host, peers []reach.Peer, r runner.Runner, base Baseliner, w waker) tea.Cmd {
	return func() tea.Msg {
		return hostRowMsg{row: probeHostWake(h, func() []reach.Peer { return peers }, r, base, w)}
	}
}

// wakeHost runs the reachability ladder for one host off the UI thread. It
// goes through the runner seam, never tea.ExecProcess: suspending the whole
// dashboard to wake one row would reintroduce the freeze this feature removes.
func wakeHost(h sshconf.Host, peers []reach.Peer, r runner.Runner, p reach.Policy) tea.Cmd {
	return func() tea.Msg {
		res := newWaker(r, p)(h, peers)
		return wakeDoneMsg{alias: h.Alias, via: res.Via, woke: res.Woke, attempts: res.Attempts}
	}
}

// sudoPrecheck decides which lane a host updates in. `sudo -n true` succeeds
// only when sudo needs no password, which is exactly the condition under which
// an update can run unattended in the background.
const sudoPrecheck = "sudo -n true"

func precheckSudo(alias string, r runner.Runner) tea.Cmd {
	return func() tea.Msg {
		_, err := r.Run(alias, sudoPrecheck)
		return precheckMsg{alias: alias, interactive: err != nil}
	}
}

// answers are the operator's pre-supplied responses to install.sh's prompts,
// collected once per wave so an unattended run never reaches a prompt nobody
// is watching. Password is memory-only: never persisted, never logged, never
// placed in argv.
type answers struct {
	sudoSecret string // memory only: never persisted, logged, or put in argv
	windows    string // y | n | s          -> WINSETUP_ANSWER
	gemini     string // yes | keep | skip  -> GEMINI_TEARDOWN_ANSWER
}

// appendSecret / trimSecret keep the secret's mutation in one place so the
// rest of the model never handles it directly.
func (a *answers) appendSecret(s string) { a.sudoSecret += s }
func (a *answers) trimSecret() {
	if n := len(a.sudoSecret); n > 0 {
		a.sudoSecret = a.sudoSecret[:n-1]
	}
}

// secretLen is what the view is allowed to know — enough to draw a mask.
func (a answers) secretLen() int { return len(a.sudoSecret) }

// needsSudo reports whether we have a credential to prime with.
func (a answers) needsSudo() bool { return a.sudoSecret != "" }

// remembered reports whether a previous wave (or the persisted preferences)
// already filled anything in. It decides whether `u` opens the form or goes
// straight to the confirm strip.
func (a answers) remembered() bool {
	return a.sudoSecret != "" || a.windows != "" || a.gemini != ""
}

// summary is what the confirm strip prints. It is the compensating control for
// answers that outlive their wave: the operator sees exactly what is about to
// be applied, every time. The credential appears ONLY as a length mask.
func (a answers) summary() string {
	parts := []string{"sudo " + maskOrNone(a.secretLen())}
	parts = append(parts, "windows "+orUnset(a.windows))
	parts = append(parts, "gemini "+orUnset(a.gemini))
	return strings.Join(parts, " · ")
}

func maskOrNone(n int) string {
	if n == 0 {
		return "(none)"
	}
	return strings.Repeat("•", n)
}

func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

// envPrefix renders the pre-answers as environment assignments for the remote
// shell. Only the two prompt answers travel this way — the password never
// does, because environment (like argv) is readable via /proc.
func (a answers) envPrefix() string {
	var b []string
	if a.windows != "" {
		b = append(b, "WINSETUP_ANSWER="+a.windows)
	}
	if a.gemini != "" {
		b = append(b, "GEMINI_TEARDOWN_ANSWER="+a.gemini)
	}
	if len(b) == 0 {
		return ""
	}
	return strings.Join(b, " ") + " "
}

// Exit codes the remote preamble uses to distinguish sudo problems from a
// genuine install failure, so a row's FAIL says which one happened.
const (
	rcSudoAuth    = 91 // the password was rejected
	rcSudoNoCache = 92 // authentication worked but the credential did not persist
)

// unattendedUpdate builds the remote script for a background update.
//
// Everything runs in ONE ssh session on purpose. sudo's timestamp is scoped
// (tty_tickets), so priming in a separate connection is not guaranteed to be
// visible to a later one — the credential is primed and consumed in the same
// session as install.sh.
//
// The prime is then VERIFIED with `sudo -n true` rather than assumed: if the
// credential did not persist we fail immediately with a distinct code instead
// of running a long install whose privileged steps all silently skip.
func unattendedUpdate(ref string, a answers) string {
	var b strings.Builder
	if a.needsSudo() {
		// -S reads the password from stdin (never argv); -p '' suppresses the
		// prompt text that would otherwise pollute the captured log.
		fmt.Fprintf(&b, "sudo -S -p '' -v 2>/dev/null || exit %d; ", rcSudoAuth)
		fmt.Fprintf(&b, "sudo -n true 2>/dev/null || exit %d; ", rcSudoNoCache)
	}
	b.WriteString(a.envPrefix())
	b.WriteString(remoteUpdateScript(ref))
	return b.String()
}

// explainExit turns the preamble's exit codes into something an operator can
// act on. A bare "exit status 91" on a row would be useless.
func explainExit(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, fmt.Sprint(rcSudoAuth)):
		return "sudo authentication failed (wrong password?)"
	case strings.Contains(s, fmt.Sprint(rcSudoNoCache)):
		return "sudo credential did not persist on this host; use the interactive lane"
	}
	return s
}

// bgUpdate runs an update WITHOUT taking the terminal, so many hosts update
// at once and the TUI stays interactive. This is the default lane; the
// ExecProcess handoff is reserved for hosts that must prompt.
//
// The password reaches the remote `sudo -S` over ssh's encrypted channel via
// stdin. It is never an argument and never an environment variable, both of
// which are world-readable through /proc.
func bgUpdate(alias, ref string, a answers, r runner.Runner) tea.Cmd {
	script := unattendedUpdate(ref, a)
	return func() tea.Msg {
		var out string
		var err error
		if a.needsSudo() {
			out, err = r.RunStdin(alias, a.sudoSecret+"\n", script)
		} else {
			out, err = r.Run(alias, script)
		}
		log := tailLines(out, 3)
		if e := explainExit(err); e != "" && err != nil {
			log = strings.TrimSpace(e + " " + log)
		}
		return bgUpdateDoneMsg{alias: alias, log: log, err: err}
	}
}

// interactiveHandoff gives the terminal away so install.sh's sudo prompt
// reaches the operator. tea.ExecProcess SUSPENDS the whole TUI, which is why
// this lane is serial and used only when the precheck says it is required.
func interactiveHandoff(alias, ref string) tea.Cmd {
	c := exec.Command("ssh", "-t", alias, remoteUpdateScript(ref))
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{alias: alias, err: err}
	})
}

// sshShell drops the operator onto the host and restores the TUI on exit.
func sshShell(alias string) tea.Cmd {
	c := exec.Command("ssh", alias)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{alias: alias, err: err, ssh: true}
	})
}

// tailLines keeps the last n non-empty lines — enough to explain a failure
// without letting a full install log into the row.
func tailLines(s string, n int) string {
	var keep []string
	for _, l := range splitNonEmpty(s) {
		keep = append(keep, l)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return joinTrim(keep)
}
