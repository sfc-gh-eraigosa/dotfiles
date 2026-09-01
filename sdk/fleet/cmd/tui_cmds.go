package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
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

	// logLineMsg carries one streamed output line from an in-flight update.
	logLineMsg struct{ alias, line string }
	// logEOFMsg says a host's stream ended; the completion arrives separately
	// on doneCh, so the two are joined in the model.
	logEOFMsg struct{ alias string }
	// streamStartedMsg hands the freshly-opened channels to the model.
	streamStartedMsg struct {
		alias string
		st    stream
	}
)

// stream is one host's in-flight output channels, parked in the model so the
// reader Cmd can be re-issued after every line.
type stream struct {
	lines <-chan string
	done  <-chan error
}

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
	reset      string // y | n — force the clone onto origin before installing
}

// forceReset reports whether this wave should hard-reset each host onto the
// fetched commit rather than fast-forwarding.
func (a answers) forceReset() bool { return a.reset == "y" }

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
	return a.sudoSecret != "" || a.windows != "" || a.gemini != "" || a.reset != ""
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
	// MUST be `export …;`, not the `VAR=x cmd` prefix form. The prefix form
	// scopes the assignment to that ONE command, so
	// `WINSETUP_ANSWER=s cd ~/git/dotfiles && ./install.sh` sets it for `cd`
	// and nothing else — install.sh never sees it and prompts anyway. Live
	// defect: the answers were collected, transmitted, and silently dropped.
	// Verified: `sh -c 'FOO=bar cd /tmp && env | grep FOO'` prints nothing.
	return "export " + strings.Join(b, " ") + "; "
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
// sudoGate refuses to start install.sh unless sudo will actually work in THIS
// session.
//
// install.sh treats its own failed `sudo -v` as non-fatal and carries on, so
// without this gate a credential-less run produced a long cascade —
//
//	sudo: a password is required
//	sudo: a terminal is required to read the password ...
//	WARNING: apt-get update failed; installs may be incomplete.
//	WARNING: grouped install failed; retrying packages individually...
//
// — and could still exit 0, leaving the row reading `ok` while every
// privileged step had silently skipped. A half-installed host that reports
// success is the worst outcome this tool can produce.
//
// It is unconditional on purpose. Gating only when a credential was supplied
// left exactly the case above ungated. It also cannot be inherited from the
// precheck: that runs in a SEPARATE ssh connection, and sudo's timestamp is
// scoped, so passing there says nothing about this session.
//
// Hosts that need no sudo are exempt rather than blocked: root, and machines
// with no sudo at all (minimal containers).
const sudoGate = `{ [ "$(id -u)" = 0 ] || ! command -v sudo >/dev/null 2>&1 || sudo -n true 2>/dev/null; }`

func unattendedUpdate(ref string, a answers) string {
	var b strings.Builder
	if a.needsSudo() {
		// -S reads the password from stdin (never argv); -p '' suppresses the
		// prompt text that would otherwise pollute the captured log.
		fmt.Fprintf(&b, "sudo -S -p '' -v 2>/dev/null || exit %d; ", rcSudoAuth)
	}
	fmt.Fprintf(&b, "%s || exit %d; ", sudoGate, rcSudoNoCache)
	b.WriteString(a.envPrefix())
	b.WriteString(updateScript(ref, a.forceReset()))
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
		return "sudo unusable in this session (no credential, or it did not persist) — nothing was installed"
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
// beginStream launches the update from inside a Cmd and hands the channels
// back as a message. Starting it in Update() instead would put I/O on the
// update path — where a model built without a runner (tests, the demo) panics,
// and where a blocking dial would freeze the UI. Update stays pure; only Cmds
// touch the network.
func beginStream(alias, ref string, a answers, r runner.Runner, dir string) tea.Cmd {
	script := unattendedUpdate(ref, a)
	secret := a.sudoSecret + "\n"
	reset := a.forceReset()
	return func() tea.Msg {
		lines, done := r.RunStream(alias, secret, script)
		// Tee to disk from HERE — inside the Cmd, off the UI thread. The pane
		// is an in-memory ring that dies with the process; the capture is what
		// survives to be read the morning after.
		lines = teeToRunLog(dir, alias, ref, reset, lines)
		return streamStartedMsg{alias: alias, st: stream{lines: lines, done: done}}
	}
}

// teeToRunLog forwards every line unchanged while writing it to this run's
// capture. The file's whole lifecycle — location, 0600, header, per-line
// timestamps, retention — belongs to libs/log; fleet only says what the run
// is about.
//
// A capture that cannot be opened is nil, and a nil capture's Tee returns the
// stream untouched: losing the log must never cost the update.
func teeToRunLog(dir, alias, ref string, reset bool, in <-chan string) <-chan string {
	mode := "fast-forward"
	if reset {
		mode = "FORCE RESET"
	}
	c := applog.NewCapture(applog.CaptureOptions{
		Tool:    logTool,
		Dir:     dir,
		Subject: alias,
		Header: fmt.Sprintf("fleet update — host=%s ref=%s mode=%s started=%s",
			alias, ref, mode, nowFn().UTC().Format(time.RFC3339)),
		Now: nowFn,
	})
	if c != nil {
		// fleet's own diagnostics now go somewhere too, not just the remote's
		// output — including where to find that output.
		applog.Default().WithFields(map[string]any{
			"host": alias, "ref": ref, "mode": mode, "capture": c.Path(),
		}).Info("update started")
	}
	return c.Tee(in, "finished")
}

// readLine blocks until the next line (or EOF) and turns it into a Msg. It is
// re-issued on every logLineMsg, which is how a channel becomes a stream of
// bubbletea messages without a goroutine writing into the model.
func readLine(alias string, st stream) tea.Cmd {
	return func() tea.Msg {
		l, ok := <-st.lines
		if !ok {
			return logEOFMsg{alias: alias}
		}
		return logLineMsg{alias: alias, line: l}
	}
}

// awaitDone blocks on the command's exit and reports it once the stream has
// drained, so a row's ok/FAIL never lands before its last log line.
func awaitDone(alias string, st stream) tea.Cmd {
	return func() tea.Msg {
		err := <-st.done
		return bgUpdateDoneMsg{alias: alias, err: err}
	}
}

// shQuote makes a string safe as a single-quoted POSIX shell word.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// handoffWrapper wraps the ssh handoff in a banner naming the host and its
// position in the queue, plus a footer carrying the exit code.
//
// tea.ExecProcess SUSPENDS the whole TUI, so while a handoff runs the screen
// is bare install.sh output with nothing on it identifying which machine you
// are looking at — or that more hosts are queued behind it.
// Extracted so the contract is testable without running ssh.
func handoffWrapper(alias, ref, remote string, pos, total int) string {
	banner := fmt.Sprintf("\\n=== fleet: updating %s -> %s   (host %d of %d) ===\\n\\n", alias, ref, pos, total)
	footer := fmt.Sprintf("\\n=== fleet: %s finished (exit %%s) — returning to the dashboard ===\\n", alias)
	return fmt.Sprintf("printf %s; ssh -t %s %s; rc=$?; printf %s \"$rc\"; exit $rc",
		shQuote(banner), shQuote(alias), shQuote(remote), shQuote(footer))
}

// interactiveHandoff gives the terminal away so install.sh's sudo prompt
// reaches the operator.
//
// It carries the pre-answers too. It previously ran the BARE update script, so
// a host routed here re-asked every question the operator had already answered.
func interactiveHandoff(alias, ref string, a answers, pos, total int) tea.Cmd {
	remote := a.envPrefix() + updateScript(ref, a.forceReset())
	c := exec.Command("sh", "-c", handoffWrapper(alias, ref, remote, pos, total))
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

// configVerbArgs builds the argv for a one-way config transfer. It exists as a
// named function so a test can assert the TUI delegates to the CLI verb rather
// than reimplementing the transfer.
func configVerbArgs(direction, alias string) []string {
	return []string{"config", direction, alias}
}

// configShell suspends the TUI and runs the config verb, mirroring how `s`
// hands over for an ssh session. The transfer prints a diff and asks for
// confirmation, neither of which survives the background lane.
func configShell(direction, alias string) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		self = "fleet"
	}
	c := exec.Command(self, configVerbArgs(direction, alias)...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{alias: alias, err: err, ssh: true}
	})
}

// tailLines keeps the last n non-empty lines — enough to explain a failure
// without letting a full install log into the row.
func tailLines(s string, n int) string {
	var keep []string
	for _, l := range splitNonEmpty(s) {
		// git emits several "hint:" lines AFTER the real error, so a naive
		// tail shows only the advice and hides the cause. Observed live as a
		// row reading: FAIL: exit status 128 hint: | hint: Disable this
		// message with "git config advice…
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "hint:") || strings.HasPrefix(low, "advice:") {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return joinTrim(keep)
}
