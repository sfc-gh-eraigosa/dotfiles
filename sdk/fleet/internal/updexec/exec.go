package updexec

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// Status is a step's (or the synthesized restore step's) outcome.
type Status string

const (
	OK        Status = "ok"
	Failed    Status = "failed"
	Skipped   Status = "skipped"
	DepFailed Status = "dependency-failed"
)

// Result is one step's outcome on one host.
type Result struct {
	Step     string
	Kind     updplan.Kind
	Status   Status
	Exit     int // 0 ok; -1 unknown; 255 = ssh transport failure
	Duration time.Duration
	Reason   string
	Notes    []string // remote lines prefixed "fleet: ", prefix stripped
	Attempts int
	TimedOut bool
}

// HostReport is one host's run of the whole plan.
type HostReport struct {
	Host    string
	Plan    string
	Started time.Time
	Results []Result
	Output  string
}

// Failed reports whether any step on this host did not finish ok.
func (h HostReport) Failed() bool {
	for _, r := range h.Results {
		if r.Status != OK {
			return true
		}
	}
	return false
}

// Err returns nil, or an error naming the first non-ok step and its reason.
func (h HostReport) Err() error {
	for _, r := range h.Results {
		if r.Status != OK {
			return fmt.Errorf("step %s: %s", r.Step, r.Reason)
		}
	}
	return nil
}

// ErrNoTerminal is returned by a StepIO lane (Background) that cannot hand
// the terminal to an interactive step.
var ErrNoTerminal = errors.New("updexec: step needs a terminal this lane cannot provide")

// ErrTransport marks an ssh-level transport failure (exit 255, or a local
// connect/dial failure), distinct from the remote command's own exit code.
var ErrTransport = errors.New("updexec: ssh transport failure")

// StepIO is the lane a step's script is sent over: batch (non-interactive,
// output captured) or interactive (terminal handed to the remote command).
type StepIO interface {
	Batch(ctx context.Context, host string, st updplan.Step, script string) (out string, err error)
	Interactive(ctx context.Context, host string, st updplan.Step, script string) error
}

// Console is the CLI StepIO lane: Batch streams over runner.RunStreamCtx and
// hands every line to Line (if set); Interactive hands the terminal to
// runner.RunInteractive. Preamble and Stdin are consulted ONLY for
// updplan.KindRun steps — a sync or gh-auth script is never prefixed with a
// sudo preamble, and never receives the operator's answers on stdin.
type Console struct {
	R        runner.Runner
	Line     func(host, line string)
	Stdin    func(st updplan.Step) string
	Preamble func(st updplan.Step) string
}

func (c Console) runScript(st updplan.Step, script string) string {
	if st.Kind != updplan.KindRun || c.Preamble == nil {
		return script
	}
	if p := c.Preamble(st); p != "" {
		return p + " && " + script
	}
	return script
}

func (c Console) runStdin(st updplan.Step) string {
	if st.Kind != updplan.KindRun || c.Stdin == nil {
		return ""
	}
	return c.Stdin(st)
}

// Batch streams script over runner.RunStreamCtx, forwarding every line to
// Line (if set) and collecting them into the returned output. An ssh exit
// of 255 is mapped to ErrTransport (wrapped so errors.Is still finds it and
// exitCode can still recover 255).
func (c Console) Batch(ctx context.Context, host string, st updplan.Step, script string) (string, error) {
	lines, done := c.R.RunStreamCtx(ctx, host, c.runStdin(st), c.runScript(st, script))
	var out []string
	for l := range lines {
		if c.Line != nil {
			c.Line(host, l)
		}
		out = append(out, l)
	}
	err := <-done
	if rawExitCode(err) == 255 {
		err = fmt.Errorf("%w: %v", ErrTransport, err)
	}
	return strings.Join(out, "\n"), err
}

// Interactive hands the terminal to runner.RunInteractive. ctx is ignored
// unless it carries a deadline, in which case the call runs in a goroutine
// so a timeout can still be reported (context.DeadlineExceeded) — the
// underlying interactive session itself is not force-killed, deliberately
// kept simple: a real terminal session hitting this path is rare, and this
// is measured, not assumed, by the interactive-timeout test.
func (c Console) Interactive(ctx context.Context, host string, st updplan.Step, script string) error {
	script = c.runScript(st, script)
	dl, ok := ctx.Deadline()
	if !ok {
		return c.R.RunInteractive(host, script)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- c.R.RunInteractive(host, script) }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Until(dl)):
		return context.DeadlineExceeded
	}
}

// Background is the TUI StepIO lane: Batch is identical to Console's, but
// Interactive always fails with ErrNoTerminal — a background update has no
// terminal to hand a step, so the executor routes such a step back to the
// interactive queue instead.
type Background struct{ Console }

func (Background) Interactive(context.Context, string, updplan.Step, string) error {
	return ErrNoTerminal
}

// Output opens a per-host log; LineWriter receives every captured line.
type Output interface {
	Open(host, header string) (LineWriter, string)
}

// LineWriter receives lines for one host's capture.
type LineWriter interface {
	Line(string)
	Close(footer string)
}

// Discard is an Output that keeps nothing — the default when Executor.Out
// is nil.
type Discard struct{}

func (Discard) Open(string, string) (LineWriter, string) { return discardWriter{}, "" }

type discardWriter struct{}

func (discardWriter) Line(string)  {}
func (discardWriter) Close(string) {}

// Executor walks a Plan's steps on one host, applying retries, cascade, and
// the local-changes/restore policy.
type Executor struct {
	IO  StepIO
	Out Output

	Now   func() time.Time    // nil -> time.Now
	Sleep func(time.Duration) // nil -> time.Sleep (backoff)
	Rand  func() float64      // nil -> math/rand (jitter)

	Local updplan.Local // "" = per-repo policy; else overrides every repo (--local / --force)

	NoRestore, Reset, NoRetry bool
	Timeout                   time.Duration // >0 overrides every batch step (--timeout)
}

func (e Executor) out() Output {
	if e.Out != nil {
		return e.Out
	}
	return Discard{}
}

func (e Executor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e Executor) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if e.Sleep != nil {
		e.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (e Executor) modeLabel() string {
	if e.Reset {
		return "FORCE RESET"
	}
	return "fast-forward"
}

// effectiveTimeout resolves a step's per-attempt deadline: an interactive
// step only ever gets a deadline the plan set explicitly (Executor.Timeout
// never touches it); a batch step's deadline is Executor.Timeout when set,
// else the step's own (already-defaulted) Timeout.
func (e Executor) effectiveTimeout(st updplan.Step) time.Duration {
	if st.Interactive {
		return st.Timeout
	}
	if e.Timeout > 0 {
		return e.Timeout
	}
	return st.Timeout
}

// RunHost walks p's steps, in Order(), against host.
func (e Executor) RunHost(host string, p updplan.Plan) HostReport {
	started := e.now()
	header := fmt.Sprintf("fleet update — host=%s plan=%s mode=%s started=%s",
		host, p.Source, e.modeLabel(), started.Format(time.RFC3339))
	w, path := e.out().Open(host, header)

	rep := HostReport{Host: host, Plan: p.Source, Started: started, Output: path}
	status := map[string]Status{}
	pending := map[string]*restoreInfo{}

	for _, st := range p.Order() {
		if blocker := e.firstStopBlocker(st, status, p); blocker != "" {
			res := Result{Step: st.ID, Kind: st.Kind, Status: DepFailed, Reason: "blocked by " + blocker}
			status[st.ID] = res.Status
			rep.Results = append(rep.Results, res)
			e.fireDueRestores(w, host, p, pending, st.ID, &rep)
			continue
		}

		res := e.runStepWithRetry(w, host, st, p)
		status[st.ID] = res.Status
		rep.Results = append(rep.Results, res)

		if st.Kind == updplan.KindSync {
			e.armAndMaybeRestoreNow(w, host, p, st, res, pending, &rep)
		}
		e.fireDueRestores(w, host, p, pending, st.ID, &rep)
	}

	w.Close("finished")
	return rep
}

// firstStopBlocker returns the id of the first need whose own status is not
// OK and whose own on_failure is "stop" (or is itself dependency-failed),
// or "" if st is not blocked. Siblings whose need failed under
// on_failure: continue are NOT blocked by that need.
func (e Executor) firstStopBlocker(st updplan.Step, status map[string]Status, p updplan.Plan) string {
	for _, need := range st.Needs {
		ns, seen := status[need]
		if !seen || ns == OK {
			continue
		}
		if ns == DepFailed {
			return need
		}
		needStep, ok := p.Step(need)
		if ok && needStep.OnFailure == updplan.OnFailureStop {
			return need
		}
	}
	return ""
}

// restoreInfo tracks what a repo's sync step recorded about the branch it
// was on before syncing, so a synthesized "<repo>.restore" step can put it
// back.
type restoreInfo struct {
	orig  string
	sha   string
	armed bool
}

func (e Executor) armAndMaybeRestoreNow(w LineWriter, host string, p updplan.Plan, st updplan.Step, res Result, pending map[string]*restoreInfo, rep *HostReport) {
	repo, ok := p.RepoOf(st)
	if !ok {
		return
	}
	pi := pending[repo.Name]
	if pi == nil {
		pi = &restoreInfo{}
	}
	switched := false
	for _, n := range res.Notes {
		if v, ok := strings.CutPrefix(n, "orig="); ok {
			pi.orig = v
		}
		if strings.HasPrefix(n, "carried stash=") {
			for _, f := range strings.Fields(n) {
				if v, ok := strings.CutPrefix(f, "stash="); ok {
					pi.sha = v
				}
				if v, ok := strings.CutPrefix(f, "from="); ok && pi.orig == "" {
					pi.orig = v
				}
			}
		}
		if strings.HasPrefix(n, "switched ") {
			switched = true
		}
	}
	if pi.sha != "" || switched {
		pi.armed = true
	}
	if !pi.armed {
		return
	}
	pending[repo.Name] = pi

	if res.Status == Failed {
		rres := e.runRestore(w, host, repo, pi)
		rep.Results = append(rep.Results, rres)
		delete(pending, repo.Name)
	}
}

// fireDueRestores runs (or skips) the synthesized restore for every pending
// repo whose LastStepUsing is exactly stepID, regardless of that step's own
// outcome — a restore must still land even when an unrelated later step
// failed.
func (e Executor) fireDueRestores(w LineWriter, host string, p updplan.Plan, pending map[string]*restoreInfo, stepID string, rep *HostReport) {
	for name, pi := range pending {
		last, ok := p.LastStepUsing(name)
		if !ok || last != stepID {
			continue
		}
		repo := p.Repos[name]
		if e.NoRestore || !repo.Restore {
			rep.Results = append(rep.Results, Result{
				Step: name + ".restore", Kind: updplan.KindSync, Status: Skipped,
				Reason: "restore disabled",
			})
		} else {
			rep.Results = append(rep.Results, e.runRestore(w, host, repo, pi))
		}
		delete(pending, name)
	}
}

// runRestore runs the synthesized "<repo>.restore" step under its FIXED
// retry policy (3 attempts, retried only on a transport failure, 5m
// per-attempt timeout) — never affected by Executor.NoRetry or Timeout.
func (e Executor) runRestore(w LineWriter, host string, repo updplan.Repo, pi *restoreInfo) Result {
	id := repo.Name + ".restore"
	call := func(ctx context.Context) (string, error) {
		script, err := RestoreScript(repo, pi.orig, pi.sha)
		if err != nil {
			return "", err
		}
		st := updplan.Step{ID: id, Kind: updplan.KindSync}
		return e.IO.Batch(ctx, host, st, script)
	}
	fixed := updplan.Retry{
		Attempts: 3,
		On:       []updplan.RetryOn{updplan.RetryOnTransport},
		Backoff:  updplan.Default().Defaults.Retry.Backoff,
	}
	return e.runWithRetry(w, id, updplan.KindSync, false, fixed, 5*time.Minute, false,
		updplan.Expect{Exit: []int{0}}, call)
}

// runStepWithRetry dispatches a plan step to its kind-specific handler.
func (e Executor) runStepWithRetry(w LineWriter, host string, st updplan.Step, p updplan.Plan) Result {
	switch st.Kind {
	case updplan.KindSync:
		return e.runSync(w, host, st, p)
	case updplan.KindRun:
		return e.runRun(w, host, st, p)
	case updplan.KindGhAuth:
		return e.runGhAuth(w, host, st)
	default:
		return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Reason: fmt.Sprintf("unknown step kind %q", st.Kind)}
	}
}

func (e Executor) runRun(w LineWriter, host string, st updplan.Step, p updplan.Plan) Result {
	var repoPtr *updplan.Repo
	if repo, ok := p.RepoOf(st); ok {
		repoPtr = &repo
	}
	call := func(ctx context.Context) (string, error) {
		script, err := RunScript(st, repoPtr)
		if err != nil {
			return "", err
		}
		if st.Interactive {
			return "", e.IO.Interactive(ctx, host, st, script)
		}
		return e.IO.Batch(ctx, host, st, script)
	}
	return e.runWithRetry(w, st.ID, st.Kind, st.Interactive, st.Retry, e.effectiveTimeout(st), e.NoRetry, st.Expect, call)
}

// errSkip marks a sync outcome that is neither ok nor a failure: the step
// is recorded Skipped, with no retry and no cascade beyond the ordinary
// dependency rules.
type errSkip struct{ reason string }

func (s errSkip) Error() string { return s.reason }

// reasonDirtySkip is spec F3's exact skip reason for a dirty clone under
// local: skip.
const reasonDirtySkip = "clone is dirty; re-run with --force to preserve local work in a rescue worktree"

func (e Executor) runSync(w LineWriter, host string, st updplan.Step, p updplan.Plan) Result {
	repo, ok := p.RepoOf(st)
	if !ok {
		return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Reason: fmt.Sprintf("unknown repo %q", st.Repo)}
	}
	local := repo.Local
	if e.Local != "" {
		local = e.Local
	}
	if e.Reset && local == updplan.LocalCarry {
		return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Reason: "--reset is incompatible with local: carry"}
	}

	call := func(ctx context.Context) (string, error) {
		pcScript, err := PrecheckScript(repo)
		if err != nil {
			return "", err
		}
		pcOut, err := e.IO.Batch(ctx, host, st, pcScript)
		if err != nil {
			return pcOut, err
		}
		state, _ := parsePrecheck(pcOut)
		switch state {
		case "missing":
			if repo.URL == "" {
				return pcOut, fmt.Errorf("updexec: repo %q: missing and no url configured", repo.Name)
			}
			cs, err := CloneScript(repo)
			if err != nil {
				return "", err
			}
			return e.IO.Batch(ctx, host, st, cs)
		case "in-progress":
			return pcOut, errSkip{"merge or rebase in progress"}
		case "dirty":
			switch local {
			case updplan.LocalSkip:
				return pcOut, errSkip{reasonDirtySkip}
			case updplan.LocalRescue:
				rs, err := RescueScript(repo)
				if err != nil {
					return "", err
				}
				if _, err := e.IO.Batch(ctx, host, st, rs); err != nil {
					return "", err
				}
				ss, err := SyncScript(repo, local, e.Reset)
				if err != nil {
					return "", err
				}
				return e.IO.Batch(ctx, host, st, ss)
			case updplan.LocalCarry:
				ss, err := SyncScript(repo, local, e.Reset)
				if err != nil {
					return "", err
				}
				return e.IO.Batch(ctx, host, st, ss)
			default:
				return pcOut, fmt.Errorf("updexec: repo %q: invalid local policy %q", repo.Name, local)
			}
		case "clean":
			ss, err := SyncScript(repo, local, e.Reset)
			if err != nil {
				return "", err
			}
			return e.IO.Batch(ctx, host, st, ss)
		default:
			return pcOut, fmt.Errorf("updexec: repo %q: unexpected precheck output %q", repo.Name, pcOut)
		}
	}

	return e.runWithRetry(w, st.ID, st.Kind, false, st.Retry, e.effectiveTimeout(st), e.NoRetry, st.Expect, call)
}

func (e Executor) runGhAuth(w LineWriter, host string, st updplan.Step) Result {
	start := e.now()
	checkOnce := func(ctx context.Context) (string, error) {
		script, err := GhAuthCheck(st.Hostname)
		if err != nil {
			return "", err
		}
		return e.IO.Batch(ctx, host, st, script)
	}

	res := e.runWithRetry(w, st.ID, st.Kind, false, st.Retry, e.effectiveTimeout(st), e.NoRetry, st.Expect, checkOnce)
	if res.Status == OK {
		return res
	}
	if res.Exit == 127 {
		res.Reason = "gh not installed"
		return res
	}

	loginScript, err := GhAuthLogin(st.Hostname)
	if err != nil {
		return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Reason: err.Error(),
			Duration: e.now().Sub(start), Attempts: res.Attempts}
	}
	if err := e.IO.Interactive(context.Background(), host, st, loginScript); err != nil {
		reason := err.Error()
		if errors.Is(err, ErrNoTerminal) {
			reason = "needs a terminal"
		}
		return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Reason: reason,
			Duration: e.now().Sub(start), Attempts: res.Attempts}
	}

	out, err2 := checkOnce(context.Background())
	exit := exitCode(err2)
	notes := parseNotes(out)
	if err2 == nil && containsInt(st.Expect.Exit, exit) {
		return Result{Step: st.ID, Kind: st.Kind, Status: OK, Exit: exit,
			Duration: e.now().Sub(start), Attempts: res.Attempts + 1, Notes: notes}
	}
	return Result{Step: st.ID, Kind: st.Kind, Status: Failed, Exit: exit,
		Reason: reasonFor(err2, false, exit, 0), Duration: e.now().Sub(start),
		Attempts: res.Attempts + 1, Notes: notes}
}

// runWithRetry runs call under Executor's clock/sleep/rand, retrying per
// retry when the failure class matches, up to a bounded number of
// attempts, each under its own per-attempt deadline.
func (e Executor) runWithRetry(
	w LineWriter, id string, kind updplan.Kind, interactive bool,
	retry updplan.Retry, timeout time.Duration, noRetry bool,
	expect updplan.Expect, call func(context.Context) (string, error),
) Result {
	maxAttempts := retry.Attempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if noRetry || interactive {
		maxAttempts = 1
	}

	start := e.now()
	var wait time.Duration
	var out string
	var callErr error
	var timedOut bool
	attempts := 0

	for n := 1; n <= maxAttempts; n++ {
		attempts = n
		header := fmt.Sprintf("=== step %s (%s)", id, kind)
		if maxAttempts > 1 {
			header += fmt.Sprintf(" attempt %d/%d", n, maxAttempts)
		}
		if n > 1 && wait > 0 {
			header += fmt.Sprintf(" (after %s)", wait)
		}
		header += " ==="
		w.Line(header)

		ctx := context.Background()
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, timeout)
		}
		out, callErr = call(ctx)
		timedOut = timeout > 0 && ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
		if cancel != nil {
			cancel()
		}

		var sk errSkip
		if errors.As(callErr, &sk) {
			return Result{Step: id, Kind: kind, Status: Skipped, Reason: sk.reason,
				Duration: e.now().Sub(start), Attempts: attempts, Notes: parseNotes(out)}
		}

		exit := exitCode(callErr)
		if !timedOut && (callErr == nil || isExitError(callErr) || errors.Is(callErr, ErrTransport)) && containsInt(expect.Exit, exit) {
			return Result{Step: id, Kind: kind, Status: OK, Exit: exit,
				Duration: e.now().Sub(start), Attempts: attempts, Notes: parseNotes(out)}
		}

		class := classify(callErr, timedOut)
		if n == maxAttempts || !matchesRetry(retry.On, class, exit) {
			break
		}
		wait = retry.Backoff.Wait(n, e.rand())
		e.sleep(wait)
	}

	exit := exitCode(callErr)
	notes := parseNotes(out)
	reason := reasonFor(callErr, timedOut, exit, timeout)
	// RestoreScript's fallback prints "fleet: restore-failed stash=<sha>
	// branch=<orig>" on any failure — surface it verbatim rather than the
	// generic "exit status N" a bare *exec.ExitError gives, since it is the
	// only place the SHA and branch a kept stash needs are named.
	for _, n := range notes {
		if strings.HasPrefix(n, "restore-failed") {
			reason = n
			break
		}
	}
	return Result{
		Step: id, Kind: kind, Status: Failed, Exit: exit,
		Reason: reason, Duration: e.now().Sub(start),
		Attempts: attempts, TimedOut: timedOut, Notes: notes,
	}
}

func (e Executor) rand() func() float64 { return e.Rand }

// --- output/exit-code plumbing ----------------------------------------------

func rawExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func isExitError(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee)
}

// exitCode maps a StepIO error to a step's exit code: nil -> 0; a wrapped
// ErrTransport -> 255; *exec.ExitError -> its code; anything else -> -1
// (unknown).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrTransport) {
		return 255
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func classify(err error, timedOut bool) string {
	switch {
	case timedOut:
		return "timeout"
	case errors.Is(err, ErrTransport):
		return "transport"
	default:
		return "exit"
	}
}

func matchesRetry(on []updplan.RetryOn, class string, exit int) bool {
	for _, o := range on {
		switch {
		case o == updplan.RetryOnAny:
			return true
		case string(o) == class:
			return true
		case class == "exit" && strings.HasPrefix(string(o), "exit:"):
			if n, err := strconv.Atoi(strings.TrimPrefix(string(o), "exit:")); err == nil && n == exit {
				return true
			}
		}
	}
	return false
}

func reasonFor(err error, timedOut bool, exit int, timeout time.Duration) string {
	switch {
	case timedOut:
		return fmt.Sprintf("timed out after %s", timeout)
	case errors.Is(err, ErrTransport):
		return "transport failure"
	case err != nil:
		return err.Error()
	default:
		return fmt.Sprintf("unexpected exit %d", exit)
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// parseNotes extracts every "fleet: " prefixed line from a script's output,
// with the prefix stripped.
func parseNotes(out string) []string {
	var notes []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "fleet: "); ok {
			notes = append(notes, v)
		}
	}
	return notes
}

// parsePrecheck extracts "state=" and "branch=" from PrecheckScript's
// output.
func parsePrecheck(out string) (state, branch string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "state=") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if v, ok := strings.CutPrefix(f, "state="); ok {
				state = v
			}
			if v, ok := strings.CutPrefix(f, "branch="); ok {
				branch = v
			}
		}
		return state, branch
	}
	return "", ""
}
