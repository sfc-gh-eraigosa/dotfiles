package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshfail"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/stamp"
	"github.com/spf13/cobra"
)

// Row is one rendered host line.
type Row struct {
	Alias  string
	Class  string
	Behind int
	Commit string
	Age    time.Time
	// Note carries a qualifier the class alone cannot express — notably a
	// stamp that exists but does not parse, which must not look identical
	// to a host that has never been stamped.
	Note string
	// Branch is the dotfiles branch checked out on the host RIGHT NOW;
	// InstalledBranch is the one it last actually installed from. They differ
	// whenever someone checked something out without re-running install.sh —
	// which is exactly the state worth seeing before you target a machine, and
	// the usual explanation for an otherwise mysterious ahead/divergent row.
	Branch, InstalledBranch string
}

// Baseliner answers "what is current, and how far off is this commit?".
type Baseliner interface {
	Head() string
	Compare(sha string) (isAncestor bool, behind int)
}

// stampPath is where install-stamp.sh writes its record.
const stampPath = "~/.local/state/dotfiles/install-stamp"

// remoteRepo is where install.sh puts the clone on every fleet host.
const remoteRepo = "~/git/dotfiles"

// probeDelim separates the two payloads the probe brings back. It must be
// something neither a stamp line nor a git branch name can contain, so the
// split can never cut in the wrong place.
const probeDelim = "__fleet_probe__"

// probeCmd reads the install stamp AND the live checked-out branch in ONE
// remote command, so knowing the branch costs no extra SSH round-trip. On a
// large fleet a second dial per host would double the poll for one column.
//
// Everything is failure-tolerant: a host with no stamp, no clone, or no git
// still answers, and the missing half simply renders as unknown.
var probeCmd = "cat " + stampPath + " 2>/dev/null; echo '" + probeDelim + "'; " +
	"git -C " + remoteRepo + " rev-parse --abbrev-ref HEAD 2>/dev/null || true"

// splitProbe separates the stamp text from the live branch. A reply with no
// delimiter is an older host answering the previous single-payload command —
// it still classifies correctly, it just has no branch to show.
func splitProbe(out string) (stampText, liveBranch string) {
	i := strings.Index(out, probeDelim)
	if i < 0 {
		return out, ""
	}
	return out[:i], strings.TrimSpace(out[i+len(probeDelim):])
}

// normalizeBranch turns git's answer into something displayable. A detached
// HEAD makes `rev-parse --abbrev-ref` print the literal "HEAD", which would
// read as a branch named HEAD.
func normalizeBranch(s string) string {
	s = strings.TrimSpace(s)
	if s == "HEAD" {
		return "detached"
	}
	return s
}

// branchCell renders the branch column. When the live branch differs from the
// one the host installed from, both are shown — that difference is the signal,
// not noise to be flattened away.
func branchCell(r Row) string {
	live := r.Branch
	if live == "" {
		return "-"
	}
	if r.InstalledBranch != "" && r.InstalledBranch != live {
		// Compact on purpose: this has to fit a table column. "live≠installed"
		// says "checked out X but installed from Y" in the width available.
		return live + "\u2260" + r.InstalledBranch
	}
	return live
}

// waker gives an unreachable host a second chance before it is written off,
// receiving the other fleet hosts as relay candidates. A nil waker disables
// wake entirely — which is both what --no-wake means and what every call site
// that predates the feature gets by construction (spec F15d).
type waker func(target sshconf.Host, peers []reach.Peer) reach.Result

// probeHost classifies ONE host. Extracted from collect so the TUI can poll
// hosts individually and stream each result as it lands (spec F1) while the
// headless path keeps probing them all at once. Both callers share this exact
// logic, so classification can never diverge between the two.
func probeHost(h sshconf.Host, r runner.Runner, base Baseliner) Row {
	return probeHostWake(h, nil, r, base, nil)
}

// probeHostWake is probeHost with the reachability ladder wired in. On a
// failed first probe it runs the ladder and, only if the ladder reports the
// host genuinely reachable again, probes a second time and records how it
// woke. `drift` is untouched: the ladder buys the probe another try, it does
// not change what any class means.
// peers is a thunk, not a slice: it must be evaluated when the ladder actually
// runs, so it reflects which hosts have answered by then. Evaluated eagerly it
// would always be empty, and the ranking would be worthless.
func probeHostWake(h sshconf.Host, peers func() []reach.Peer, r runner.Runner, base Baseliner, w waker) Row {
	row := Row{Alias: h.Alias}
	out, err := r.Run(h.Alias, probeCmd)

	// A host that answered SSH and then refused us is not asleep, so the
	// ladder has nothing to fix: running it would spend a full wake budget
	// per host, every run, on ICMP and relay hops that cannot help.
	authFailed := sshfail.Classify(err) == sshfail.Auth
	if authFailed {
		row.Note = sshfail.Note(err)
	}

	if err != nil && w != nil && !authFailed {
		var ps []reach.Peer
		if peers != nil {
			ps = peers()
		}
		if res := w(h, ps); res.Woke {
			if out2, err2 := r.Run(h.Alias, probeCmd); err2 == nil {
				out, err = out2, err2
				row.Note = "woke via " + res.Via
			}
		}
	}
	stampText, liveBranch := splitProbe(out)
	row.Branch = normalizeBranch(liveBranch)
	in := drift.Input{Reachable: err == nil, AuthFailed: authFailed, Baseline: base.Head()}
	if err == nil {
		s, perr := stamp.Parse(stampText)
		switch {
		case perr == nil:
			in.HaveStamp = true
			in.Commit = s.Commit
			in.IsAncestor, in.BehindCount = base.Compare(s.Commit)
			row.Commit = short(s.Commit)
			row.Age = s.InstalledAt
			row.InstalledBranch = s.Branch
		case strings.TrimSpace(stampText) != "":
			// A stamp file exists but does not parse — a truncated or
			// corrupted write. Reporting this as a plain "unknown"
			// would hide a real problem behind "never installed".
			// Appended, not assigned: a host can both have woken and have a
			// corrupt stamp, and losing either fact hides a real problem.
			row.Note = strings.TrimPrefix(row.Note+"; corrupt stamp", "; ")
		}
	}
	res := drift.Classify(in)
	row.Class, row.Behind = string(res.Class), res.Behind
	return row
}

// collect probes every host concurrently and classifies it. Pure apart from
// the injected runner and baseliner, so it is fully unit-testable.
func collect(hosts []sshconf.Host, r runner.Runner, base Baseliner, now time.Time) []Row {
	return collectWake(hosts, r, base, now, nil)
}

// collectWake is collect with the reachability ladder wired in. The ladder
// runs INSIDE this fan-out, not after it, so N sleeping hosts cost about one
// wake budget of wall clock rather than N (spec F15c) — the property that
// makes auto-wake affordable by default.
func collectWake(hosts []sshconf.Host, r runner.Runner, base Baseliner, now time.Time, w waker) []Row {
	rows := make([]Row, len(hosts))
	track := &liveHosts{up: map[string]bool{}}

	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h sshconf.Host) {
			defer wg.Done()
			peers := func() []reach.Peer { return track.peersFor(h.Alias, hosts) }
			rows[i] = probeHostWake(h, peers, r, base, w)
			// Only a host that actually ANSWERED may serve as a relay. One
			// that refused us is reachable at the IP layer yet cannot run a
			// command, so ranking it live would send a straggler's ladder
			// through a hop guaranteed to fail.
			if c := rows[i].Class; c != string(drift.Unreachable) && c != string(drift.AuthFailed) {
				track.markUp(h.Alias)
			}
		}(i, h)
	}
	wg.Wait()
	return rows
}

// liveHosts records which hosts have answered a direct probe in THIS run. A
// straggler's ladder consults it to prefer a peer already known to be awake —
// relaying through a second sleeping host would turn one slow host into two.
// The fan-out means the fast hosts have usually answered by the time a
// straggler's SSH timeout expires.
type liveHosts struct {
	mu sync.Mutex
	up map[string]bool
}

func (l *liveHosts) markUp(alias string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.up[alias] = true
}

// peersFor returns every fleet host except the target, stamped with whether it
// has already answered in this run. It is evaluated lazily at ladder time, so
// the reachability snapshot is as fresh as the fan-out can make it.
func (l *liveHosts) peersFor(target string, all []sshconf.Host) []reach.Peer {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]reach.Peer, 0, len(all))
	for _, h := range all {
		if h.Alias == target {
			continue
		}
		name := h.HostName
		if name == "" {
			name = h.Alias
		}
		out = append(out, reach.Peer{Alias: h.Alias, HostName: name, Reachable: l.up[h.Alias]})
	}
	return out
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

var severity = map[string]int{
	string(drift.Unreachable): 0,
	string(drift.AuthFailed):  1,
	string(drift.Divergent):   2,
	string(drift.Unknown):     3,
	string(drift.Behind):      4,
	string(drift.UpToDate):    5,
}

func statusLabel(r Row) string {
	if r.Class == string(drift.Behind) {
		return fmt.Sprintf("behind %d", r.Behind)
	}
	return r.Class
}

func sortWorstFirst(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severity[rows[i].Class] != severity[rows[j].Class] {
			return severity[rows[i].Class] < severity[rows[j].Class]
		}
		return rows[i].Alias < rows[j].Alias
	})
}

// sortByAlias orders rows by host name and nothing else.
//
// The TUI uses this rather than sortWorstFirst because severity ordering is
// UNSTABLE while rows stream in: a host's class changes from polling to its
// real verdict, its severity changes with it, and the row jumps position under
// the operator's eyes. The alias-keyed cursor survives that, but a list that
// reshuffles as you read it is unusable. A one-shot report has no such problem,
// so `fleet status` keeps worst-first.
func sortByAlias(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Alias < rows[j].Alias })
}

func renderTable(rows []Row, now time.Time) string {
	sortWorstFirst(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %-9s %-22s %-13s %s\n", "HOST", "COMMIT", "BRANCH", "LAST RUN", "STATUS")
	for _, r := range rows {
		commit := r.Commit
		if commit == "" {
			commit = "-"
		}
		status := statusLabel(r)
		if r.Note != "" {
			status += " (" + r.Note + ")"
		}
		fmt.Fprintf(&b, "%-16s %-9s %-22s %-13s %s\n", r.Alias, commit, branchCell(r), drift.FormatAge(now, r.Age), status)
	}
	return b.String()
}

func renderJSON(rows []Row) string {
	sortWorstFirst(rows)
	type jsonRow struct {
		Alias           string `json:"alias"`
		Status          string `json:"status"`
		Behind          int    `json:"behind"`
		Commit          string `json:"commit"`
		Branch          string `json:"branch,omitempty"`
		InstalledBranch string `json:"installed_branch,omitempty"`
		Note            string `json:"note,omitempty"`
		InstalledAt     string `json:"installed_at,omitempty"`
	}
	out := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		j := jsonRow{
			Alias: r.Alias, Status: r.Class, Behind: r.Behind, Commit: r.Commit,
			Branch: r.Branch, InstalledBranch: r.InstalledBranch, Note: r.Note,
		}
		if !r.Age.IsZero() {
			j.InstalledAt = r.Age.UTC().Format(time.RFC3339)
		}
		out = append(out, j)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}

// exitError carries a process exit code out of RunE. Calling os.Exit inside
// a command bypasses cobra's error handling and any deferred cleanup, and
// makes the stale path impossible to test end-to-end.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("fleet: %d host(s) not up to date", e.code) }

// exitErrorFor returns a non-nil exitError when any host is not up to date.
func exitErrorFor(rows []Row) error {
	if exitCode(rows) != 0 {
		return exitError{code: 1}
	}
	return nil
}

// exitCode is non-zero when any host is not up to date, so `fleet status`
// works as a scripted check (spec F5).
func exitCode(rows []Row) int {
	for _, r := range rows {
		if r.Class != string(drift.UpToDate) {
			return 1
		}
	}
	return 0
}

// gitBaseline resolves origin/<branch> in the local dotfiles clone.
type gitBaseline struct {
	repo, ref, head string
}

func newGitBaseline(repo, ref string) (*gitBaseline, error) {
	// A failed fetch is not fatal (fleet must work offline) but it MUST be
	// reported: classifying against a stale origin/main can call a host
	// up-to-date when it is not, and silence there is indistinguishable
	// from a correct answer.
	if err := exec.Command("git", "-C", repo, "fetch", "-q", "origin").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch %s (%v) — baseline may be stale\n", repo, err)
	}
	out, err := exec.Command("git", "-C", repo, "rev-parse", ref).Output()
	if err != nil {
		return nil, fmt.Errorf("resolving %s in %s: %w", ref, repo, err)
	}
	return &gitBaseline{repo: repo, ref: ref, head: strings.TrimSpace(string(out))}, nil
}

func (g *gitBaseline) Head() string { return g.head }

func (g *gitBaseline) Compare(sha string) (bool, int) {
	if exec.Command("git", "-C", g.repo, "merge-base", "--is-ancestor", sha, g.ref).Run() != nil {
		return false, 0
	}
	out, err := exec.Command("git", "-C", g.repo, "rev-list", "--count", sha+".."+g.ref).Output()
	if err != nil {
		return true, 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return true, n
}

var statusCmd = &cobra.Command{
	Use:           "status [host...]",
	Short:         "Show which hosts are out of sync with the latest dotfiles install",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := os.ReadFile(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
		}
		hosts := selectHosts(all, args)
		if len(hosts) == 0 {
			if len(args) > 0 {
				return fmt.Errorf("unknown host(s) %s in %s", strings.Join(args, ", "), flagConfig)
			}
			return fmt.Errorf("no fleet hosts found — mark hosts with %q in %s, or pass aliases explicitly", flagMarker, flagConfig)
		}
		base, err := newGitBaseline(flagRepo, flagRef)
		if err != nil {
			return err
		}
		p, err := wakePolicy()
		if err != nil {
			return err
		}
		r := runner.Exec{}
		rows := collectWake(hosts, r, base, time.Now(), newWaker(r, p))
		if flagJSON {
			fmt.Fprintln(cmd.OutOrStdout(), renderJSON(rows))
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "baseline: %s %s\n\n", flagRef, short(base.Head()))
			fmt.Fprint(cmd.OutOrStdout(), renderTable(rows, time.Now()))
		}
		return exitErrorFor(rows)
	},
}

// selectHosts returns the explicitly named hosts, or every marked host.
func selectHosts(all []sshconf.Host, args []string) []sshconf.Host {
	if len(args) == 0 {
		var out []sshconf.Host
		for _, h := range all {
			if h.Fleet {
				out = append(out, h)
			}
		}
		return out
	}
	want := map[string]bool{}
	for _, a := range args {
		want[a] = true
	}
	var out []sshconf.Host
	for _, h := range all {
		if want[h.Alias] {
			out = append(out, h)
		}
	}
	return out
}

var flagRef string

func init() {
	statusCmd.Flags().StringVar(&flagRef, "ref", "origin/main", "baseline git ref")
	rootCmd.AddCommand(statusCmd)
}
