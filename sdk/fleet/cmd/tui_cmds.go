package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
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

	// logLineMsg carries one streamed output line from an in-flight update,
	// tagged with the stream that produced it.
	logLineMsg struct {
		alias, line string
		stderr      bool
	}
	// logEOFMsg says a host's stream ended; the completion arrives separately
	// on doneCh, so the two are joined in the model.
	logEOFMsg struct{ alias string }
	// streamStartedMsg hands the freshly-opened channels to the model.
	streamStartedMsg struct {
		alias string
		st    stream
	}
)

// outLine is one streamed line plus the stream it came from — the tag has to
// survive the queue and the channel, or the error pane has nothing to filter on.
type outLine struct {
	text   string
	stderr bool
}

// stream is one host's in-flight output channels, parked in the model so the
// reader Cmd can be re-issued after every line.
type stream struct {
	lines <-chan outLine
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

// envWinsetupAnswer / envGeminiTeardownAnswer are the ONE definition of the
// two install.sh prompt-answer environment variable names — envPrefix,
// handoffEnv, and localAnswerPreamble (update_output.go) each used to spell
// these out as separate string literals, three copies that could silently
// drift from one another.
const (
	envWinsetupAnswer       = "WINSETUP_ANSWER"
	envGeminiTeardownAnswer = "GEMINI_TEARDOWN_ANSWER"
)

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
		b = append(b, envWinsetupAnswer+"="+a.windows)
	}
	if a.gemini != "" {
		b = append(b, envGeminiTeardownAnswer+"="+a.gemini)
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

// sudoGate refuses to start a run step's script unless sudo will actually
// work for install.sh's OWN CHILDREN in THIS session.
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
// precheck: that runs in a SEPARATE ssh connection, and sudo's default
// timestamp_type=tty has no tty to key on over ssh, so it falls back to the
// PPID of whichever process ran `sudo` — a precheck passing there says
// nothing about this session's PPID.
//
// The check MUST run from a forked child (`sh -c '...'`), not directly in
// the preamble's own shell. Without the fork, the gate shares the primer's
// exact PPID and always reports success — even on a host where the
// credential does NOT reach install.sh's own children (pkg-install-apt, the
// keep-alive loop, the docker step), each spawned with install.sh's PPID,
// not the preamble shell's. That mismatch is exactly what let a
// credential-less run above look primed while every privileged step inside
// install.sh still failed (36x per run, live on a host with no tty and no
// NOPASSWD). Forking one level down puts the test in the same PPID shape as
// those children, so it actually answers "will THEY see this credential".
//
// The `; exit $?` after `sudo -n true` is load-bearing: a shell given a
// single simple command via `-c` may exec it in place instead of forking
// (bash does — and bash is /bin/sh on macOS, Fedora, Arch), which would give
// sudo the preamble shell as its parent again and silently restore the
// same-PPID false pass. A trailing command forces a real fork everywhere.
//
// A host with `Defaults timestamp_type=global` in sudoers (or NOPASSWD) has
// no PPID scoping to trip on and passes the child check exactly like it
// passed the old same-shell one — such a host keeps the background lane.
//
// Hosts that need no sudo are exempt rather than blocked: root, and machines
// with no sudo at all (minimal containers).
const sudoGate = `{ [ "$(id -u)" = 0 ] || ! command -v sudo >/dev/null 2>&1 || sh -c 'sudo -n true; exit $?' 2>/dev/null; }`

// errSudoNotInherited marks a background run the sudoGate stopped with its
// rcSudoNoCache exit (92): the credential primed this session did not reach
// the child-process check, so it would not reach install.sh's own children
// either. tui_model routes such a host to the interactive queue (a real pty,
// where sudo's tty keying works normally) instead of marking it failed.
var errSudoNotInherited = errors.New("fleet: sudo credential does not reach child processes")

// bgDoneErr turns a background run's report into the error its done channel
// carries, typed for the two outcomes the model re-routes rather than fails.
// The gate is recognised by the failing step's TYPED exit code, never by
// matching "92" in text — that also matched exit 192 and any reason that
// merely mentioned a 192.168.x address.
func bgDoneErr(rep updexec.HostReport) error {
	if rep.NeedsTerminal() {
		return errNeedsTerminal
	}
	for _, r := range rep.Results {
		if r.Status != updexec.OK {
			if r.Exit == rcSudoNoCache {
				return fmt.Errorf("%w: %w", errSudoNotInherited, rep.Err())
			}
			break
		}
	}
	return rep.Err()
}

// sudoGateFailed reports whether a background run's error is the sudoGate's
// (see errSudoNotInherited).
func sudoGateFailed(err error) bool {
	return errors.Is(err, errSudoNotInherited)
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
// errNeedsTerminal marks a background run that stopped on a step the
// Background lane cannot service (updexec.ErrNoTerminal) — the model routes
// such a host to the interactive queue instead of marking its row failed
// (see tuiModel.Update's bgUpdateDoneMsg case).
var errNeedsTerminal = errors.New("fleet: this host's plan needs a terminal")

// bgPolicy is the operator's standing choices for the background lane, read
// from gff once at startup (bgPolicyFromFlags) — policy, not per-wave
// answers, so it lives on the model rather than in `answers`, which the form
// and the answers store replace wholesale.
type bgPolicy struct {
	// sudoTimestampGlobal enables sudoGlobalFixup: fleet.update.sudo-timestamp-global
	// (on by default; false opts a centrally-managed host out).
	sudoTimestampGlobal bool
}

// bgPolicyFromFlags resolves the lane policy; every field is fail-closed
// (featflag.Settings.SudoTimestampGlobal), so no gff means no host changes.
func bgPolicyFromFlags(src featflag.Source, repoDir string) bgPolicy {
	return bgPolicy{sudoTimestampGlobal: featflag.Resolve(src, "", repoDir).SudoTimestampGlobal}
}

// sudoersDropIn is the file sudoGlobalFixup installs. Deleting it undoes the
// change; the name has no '.' or '~', which sudoers.d would ignore.
const sudoersDropIn = "/etc/sudoers.d/fleet-timestamp"

// sudoGlobalFixup replaces the plain prime unless the operator opted out
// (fleet.update.sudo-timestamp-global, on by default — it fires only once a
// password was typed for the host, is announced, and one file undoes it): on
// a host where the primed credential
// does not reach install.sh's children, it installs a sudoers drop-in so it
// does, and the host stays in the streaming lane instead of dropping to the
// interactive one and re-prompting for a password already typed.
//
// Why this and not a credential bridge: the security review compared
// forwarding the password to the children (an askpass FIFO) with changing
// sudo's timestamp scope, and the asymmetry was decisive — a compromised step
// inside install.sh's tree (an npm postinstall, a plugin update) would read
// the reusable PLAINTEXT password from a bridge, whereas with
// timestamp_type=global it gains root on that one host for the sudo timeout,
// which it could already get from a primed sudo. The password never leaves
// the priming shell: it is read from stdin into a plain variable (never
// exported, never a file, never argv — printf is a builtin) and expanded only
// into a pipe to `sudo -S`, with xtrace switched off first so a `set -x` from
// a sourced startup file cannot print it into the captured log.
//
// Order: prime and verify (exit rcSudoAuth on a bad password, before anything
// else); a first child check exactly like the gate's; only when it fails —
// the priming shell's own sudo calls DO carry the credential — write
// `Defaults:<user> timestamp_type=global` to a temp file, have `visudo -cf`
// vet it, install it 0440 root, say so on the run's output, and re-prime so a
// global-scoped timestamp exists. Then the ordinary gate decides the lane; if
// any of that failed (sudoers.d not included, visudo refused) nothing was
// installed and the host takes today's interactive reroute.
//
// POSIX sh: the remote login shell runs this, and that is zsh on some hosts.
var sudoGlobalFixup = "set +x 2>/dev/null; _fs_pw=$(cat); " +
	fmt.Sprintf("printf '%%s\\n' \"$_fs_pw\" | sudo -S -p '' -v 2>/dev/null || { unset _fs_pw; exit %d; }; ", rcSudoAuth) +
	"if ! " + sudoGate + "; then " +
	"_fs_u=$(id -un); _fs_f=$(mktemp \"${TMPDIR:-/tmp}/fleet-sudoers.XXXXXX\") && " +
	"printf 'Defaults:%s timestamp_type=global\\n' \"$_fs_u\" > \"$_fs_f\" && " +
	"sudo visudo -cf \"$_fs_f\" >/dev/null 2>&1 && " +
	"sudo install -m 0440 -o root \"$_fs_f\" " + sudoersDropIn + " && " +
	"echo \"fleet: the sudo credential did not reach child processes; installed " + sudoersDropIn +
	" (Defaults:$_fs_u timestamp_type=global) per fleet.update.sudo-timestamp-global - delete that file to undo\" && " +
	"printf '%s\\n' \"$_fs_pw\" | sudo -S -p '' -v 2>/dev/null; " +
	"rm -f \"$_fs_f\"; fi; unset _fs_pw _fs_u _fs_f; "

// bgPreamble is bgPreambleWith under the default (all-off) policy — what the
// tests and the dry-run path use.
func bgPreamble(a answers) func(updplan.Step) string { return bgPreambleWith(a, bgPolicy{}) }

// bgPreambleWith builds the Background lane's per-run-step preamble: prime
// and verify sudo (only when a credential was supplied — an empty `sudo -S`
// would consume nothing and fail confusingly), via sudoGlobalFixup unless the
// operator opted out; then the sudoGate every run must pass regardless; then
// the operator's non-secret answers. Console and Background apply this ONLY
// to updplan.KindRun steps, so a sync or gh-auth script never sees a sudo
// preamble.
func bgPreambleWith(a answers, p bgPolicy) func(updplan.Step) string {
	return func(updplan.Step) string {
		var b strings.Builder
		switch {
		case a.needsSudo() && p.sudoTimestampGlobal:
			b.WriteString(sudoGlobalFixup)
		case a.needsSudo():
			fmt.Fprintf(&b, "sudo -S -p '' -v 2>/dev/null || exit %d; ", rcSudoAuth)
		}
		fmt.Fprintf(&b, "%s || exit %d; ", sudoGate, rcSudoNoCache)
		b.WriteString(a.envPrefix())
		return b.String()
	}
}

// lineQueue is an unbounded, mutex-guarded FIFO decoupling a producer that
// must NEVER block (the executor's Line callback) from a consumer that may
// stall for an arbitrarily long time (readLine's Cmd, which is only
// re-issued from Update() — and Update() itself stops running entirely
// while tea.ExecProcess suspends the event loop for another host's
// interactive handoff). Before this, Line pushed onto a capacity-64
// channel: once a fast background install filled it with nobody reading,
// the executor's own goroutine — and with it, the whole update — stalled
// until the UI got back around to draining. With a 30-minute per-attempt
// deadline on batch steps, a long enough stall could kill the install
// mid-flight for a reason that had nothing to do with the remote host.
type lineQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []outLine
	closed bool
}

func newLineQueue() *lineQueue {
	q := &lineQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// push never blocks: it only grows a slice and signals, so the executor's
// Line callback returns immediately regardless of whether anything is
// reading.
func (q *lineQueue) push(l outLine) {
	q.mu.Lock()
	q.buf = append(q.buf, l)
	q.mu.Unlock()
	q.cond.Signal()
}

// closeQ marks the queue closed: once drained, forward will stop.
func (q *lineQueue) closeQ() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Signal()
}

// forward drains q into ch, in order, blocking on the channel send when
// nobody is reading — safe here because forward runs in its own goroutine,
// never on the executor's call path. It closes ch once q is closed and
// fully drained, exactly like closing a plain channel would.
func (q *lineQueue) forward(ch chan<- outLine) {
	for {
		q.mu.Lock()
		for len(q.buf) == 0 && !q.closed {
			q.cond.Wait()
		}
		if len(q.buf) == 0 {
			q.mu.Unlock()
			close(ch)
			return
		}
		l := q.buf[0]
		q.buf = q.buf[1:]
		q.mu.Unlock()
		ch <- l
	}
}

// beginStream launches the update from inside a Cmd and hands the channels
// back as a message. Starting it in Update() instead would put I/O on the
// update path — where a model built without a runner (tests, the demo) panics,
// and where a blocking dial would freeze the UI. Update stays pure; only Cmds
// touch the network.
//
// It runs plan's steps through the SAME plan executor the headless `fleet
// update` uses (updexec.Executor), over the Background StepIO lane: batch
// steps stream exactly as before, and an interactive step this lane cannot
// service (gh-auth) fails with updexec.ErrNoTerminal rather than hanging.
//
// The executor tees every line it sends into the capture (Output/LineWriter)
// itself now (updexec.Executor.RunHost), so beginStream no longer has to —
// its own Line callback only has to feed the UI's log pane via lineQueue.
func beginStream(alias string, plan updplan.Plan, a answers, r runner.Runner, dir string) tea.Cmd {
	return beginStreamWith(alias, plan, a, bgPolicy{}, r, dir)
}

// beginStreamWith is beginStream under an explicit lane policy (the model
// passes its own; see bgPolicy).
func beginStreamWith(alias string, plan updplan.Plan, a answers, p bgPolicy, r runner.Runner, dir string) tea.Cmd {
	secret := a.sudoSecret + "\n"
	preamble := bgPreambleWith(a, p)
	reset := a.forceReset()
	return func() tea.Msg {
		lines := make(chan outLine)
		done := make(chan error, 1)

		q := newLineQueue()
		go q.forward(lines)

		ex := updexec.Executor{
			IO: updexec.Background{Console: updexec.Console{
				R:       r,
				Line:    func(_, l string) { q.push(outLine{text: l}) },
				ErrLine: func(_, l string) { q.push(outLine{text: l, stderr: true}) },
				Stdin: func(st updplan.Step) string {
					if st.Kind != updplan.KindRun {
						return ""
					}
					return secret
				},
				Preamble: preamble,
			}},
			Out:   captureOutput{dir: dir},
			Reset: reset,
		}

		go func() {
			rep := ex.RunHost(alias, plan)
			q.closeQ()
			done <- bgDoneErr(rep)
		}()

		return streamStartedMsg{alias: alias, st: stream{lines: lines, done: done}}
	}
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
		return logLineMsg{alias: alias, line: l.text, stderr: l.stderr}
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

// shQuote makes a string safe as a single-quoted POSIX shell word. Aliased
// to updexec.ShQuote — the one definition both packages share, rather than
// cmd carrying its own byte-identical copy.
var shQuote = runner.Quote

// handoffWrapper wraps cmd — a full local shell command line — in a banner
// naming the host and its position in the queue, plus a footer carrying the
// exit code.
//
// tea.ExecProcess SUSPENDS the whole TUI, so while a handoff runs the screen
// is bare command output with nothing on it identifying which machine you
// are looking at — or that more hosts are queued behind it. Extracted so
// the contract is testable without running a subprocess.
func handoffWrapper(alias, ref, cmd string, pos, total int) string {
	banner := fmt.Sprintf("\\n=== fleet: updating %s -> %s   (host %d of %d) ===\\n\\n", alias, ref, pos, total)
	footer := fmt.Sprintf("\\n=== fleet: %s finished (exit %%s) — returning to the dashboard ===\\n", alias)
	return fmt.Sprintf("printf %s; %s; rc=$?; printf %s \"$rc\"; exit $rc",
		shQuote(banner), cmd, shQuote(footer))
}

// handoffArgv builds the self-exec argv for the interactive handoff:
// `<self> update <alias> [--file F] [--ref R] --repo REPO [--reset]`. There
// is now exactly ONE definition of "update a host" — the plan executor
// behind `fleet update` — so the interactive lane delegates to that CLI
// verb rather than re-implementing a remote script. Extracted so a test can
// assert on it without running a subprocess.
//
// --repo is forwarded ALWAYS (unlike --file/--ref, which are only appended
// when set): the persistent --repo flag always carries a value (it defaults
// to ~/git/dotfiles), and the TUI loaded its plan and gff resolution
// against THAT checkout. Without forwarding it, a routed host used to
// resolve gff/the repo-local plan against the child's own --repo default
// instead — possibly a different checkout than the one the TUI itself
// loaded from. --config/--marker are irrelevant to `update` and are
// deliberately left unforwarded.
func handoffArgv(self, alias, file, ref, repo string, reset bool) []string {
	argv := []string{self, "update", alias}
	if file != "" {
		argv = append(argv, "--file", file)
	}
	if ref != "" {
		argv = append(argv, "--ref", ref)
	}
	argv = append(argv, "--repo", repo)
	if reset {
		argv = append(argv, "--reset")
	}
	return argv
}

// handoffEnv is the child's environment: the operator's non-secret prompt
// answers only (WINSETUP_ANSWER/GEMINI_TEARDOWN_ANSWER) layered onto the
// parent's own environment — NEVER the sudo secret. The child prompts for
// its own credential on its own tty (it owns the terminal), so it has no
// need to receive one from the parent, and /proc/<pid>/environ is
// world-readable on both ends.
func handoffEnv(a answers) []string {
	env := os.Environ()
	if a.windows != "" {
		env = append(env, envWinsetupAnswer+"="+a.windows)
	}
	if a.gemini != "" {
		env = append(env, envGeminiTeardownAnswer+"="+a.gemini)
	}
	return env
}

// interactiveHandoff gives the terminal away so install.sh's sudo prompt
// reaches the operator, by self-execing `fleet update <alias>` — the same
// plan executor the background lane and the headless CLI use. selfFn
// resolves the executable path (os.Executable in production; injected in
// tests), matching configShell's self-exec pattern.
func interactiveHandoff(selfFn func() (string, error), alias, file, ref, repo string, a answers, pos, total int) tea.Cmd {
	self, err := selfFn()
	if err != nil || self == "" {
		self = "fleet"
	}
	argv := handoffArgv(self, alias, file, ref, repo, a.forceReset())
	quoted := make([]string, len(argv))
	for i, s := range argv {
		quoted[i] = shQuote(s)
	}
	c := exec.Command("sh", "-c", handoffWrapper(alias, ref, strings.Join(quoted, " "), pos, total))
	c.Env = handoffEnv(a)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{alias: alias, err: err}
	})
}

// sshShell drops the operator onto the host and restores the TUI on exit.
func sshShell(alias string) tea.Cmd {
	// The SAME multiplexing options the runner uses, so the connection the
	// operator authenticates here becomes the master every later batch command
	// rides. Without this the interactive session would open its own socket
	// and the next probe would prompt all over again — the whole point of
	// pressing `s` first.
	c := exec.Command("ssh", append(runner.MuxArgs(), alias)...)
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

// authorizeArgs builds the ssh-copy-id invocation. It offers a PUBLIC key and
// carries no credential mechanism of its own: no sshpass, no SSH_ASKPASS, no
// password in argv. The operator authenticates to ssh directly.
func authorizeArgs(alias string) []string {
	return []string{"ssh-copy-id", "-i", os.ExpandEnv("$HOME/.ssh/id_ed25519.pub"), alias}
}

// authorizeShell suspends the TUI so ssh-copy-id owns the terminal and can
// prompt for a password itself. On return the host is re-probed, so the row
// reflects what changed rather than the stale auth-failed verdict.
func authorizeShell(alias string) tea.Cmd {
	argv := authorizeArgs(alias)
	c := exec.Command(argv[0], argv[1:]...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{alias: alias, err: err, ssh: true}
	})
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
