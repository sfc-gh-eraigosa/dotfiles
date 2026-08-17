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
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
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
}

// Baseliner answers "what is current, and how far off is this commit?".
type Baseliner interface {
	Head() string
	Compare(sha string) (isAncestor bool, behind int)
}

// stampPath is where install-stamp.sh writes its record.
const stampPath = "~/.local/state/dotfiles/install-stamp"

// probeHost classifies ONE host. Extracted from collect so the TUI can poll
// hosts individually and stream each result as it lands (spec F1) while the
// headless path keeps probing them all at once. Both callers share this exact
// logic, so classification can never diverge between the two.
func probeHost(h sshconf.Host, r runner.Runner, base Baseliner) Row {
	row := Row{Alias: h.Alias}
	out, err := r.Run(h.Alias, "cat "+stampPath+" 2>/dev/null || true")
	in := drift.Input{Reachable: err == nil, Baseline: base.Head()}
	if err == nil {
		s, perr := stamp.Parse(out)
		switch {
		case perr == nil:
			in.HaveStamp = true
			in.Commit = s.Commit
			in.IsAncestor, in.BehindCount = base.Compare(s.Commit)
			row.Commit = short(s.Commit)
			row.Age = s.InstalledAt
		case strings.TrimSpace(out) != "":
			// A stamp file exists but does not parse — a truncated or
			// corrupted write. Reporting this as a plain "unknown"
			// would hide a real problem behind "never installed".
			row.Note = "corrupt stamp"
		}
	}
	res := drift.Classify(in)
	row.Class, row.Behind = string(res.Class), res.Behind
	return row
}

// collect probes every host concurrently and classifies it. Pure apart from
// the injected runner and baseliner, so it is fully unit-testable.
func collect(hosts []sshconf.Host, r runner.Runner, base Baseliner, now time.Time) []Row {
	rows := make([]Row, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h sshconf.Host) {
			defer wg.Done()
			rows[i] = probeHost(h, r, base)
		}(i, h)
	}
	wg.Wait()
	return rows
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

var severity = map[string]int{
	string(drift.Unreachable): 0,
	string(drift.Divergent):   1,
	string(drift.Unknown):     2,
	string(drift.Behind):      3,
	string(drift.UpToDate):    4,
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

func renderTable(rows []Row, now time.Time) string {
	sortWorstFirst(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %-9s %-13s %s\n", "HOST", "COMMIT", "LAST RUN", "STATUS")
	for _, r := range rows {
		commit := r.Commit
		if commit == "" {
			commit = "-"
		}
		status := statusLabel(r)
		if r.Note != "" {
			status += " (" + r.Note + ")"
		}
		fmt.Fprintf(&b, "%-16s %-9s %-13s %s\n", r.Alias, commit, drift.FormatAge(now, r.Age), status)
	}
	return b.String()
}

func renderJSON(rows []Row) string {
	sortWorstFirst(rows)
	type jsonRow struct {
		Alias       string `json:"alias"`
		Status      string `json:"status"`
		Behind      int    `json:"behind"`
		Commit      string `json:"commit"`
		Note        string `json:"note,omitempty"`
		InstalledAt string `json:"installed_at,omitempty"`
	}
	out := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		j := jsonRow{Alias: r.Alias, Status: r.Class, Behind: r.Behind, Commit: r.Commit, Note: r.Note}
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
		rows := collect(hosts, runner.Exec{}, base, time.Now())
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

// drift0 renders a row's install age.
func drift0(now time.Time, r Row) string { return drift.FormatAge(now, r.Age) }
