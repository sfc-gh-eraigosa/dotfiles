package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/histindex"
	"github.com/spf13/cobra"
)

var (
	flagHistoryShow     bool
	flagHistoryRun      int
	flagHistoryErrors   bool
	flagHistoryGrep     string
	flagHistoryLimit    int
	flagHistoryProblems bool
)

// historyDefaultLimit caps the default listing. Retention keeps 50 runs per
// host, so an unbounded list on a five-host fleet is 250 rows — past the
// point where scrolling beats filtering.
const historyDefaultLimit = 20

var historyCmd = &cobra.Command{
	Use:   "history [host]",
	Short: "List and read the captures past updates left behind",
	Long: `Every ` + "`fleet update`" + ` — from the CLI or the dashboard — is captured to a
per-host file under the state directory. history lists those runs and prints them.

Naming a host narrows the list to that host. --show prints a run's content
instead of listing (--run picks which, 1 = newest), --errors keeps only the
lines the remote wrote to STDERR, and --grep keeps only lines matching a
regular expression.

The stderr projection and the warning counts use the SAME classifier the
dashboard's error pane does, so the two can never disagree about what counts
as an error: git writes its whole fetch progress to stderr, and those lines
are shown but not counted.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistory(cmd.OutOrStdout(), args, fleetLogDir(), time.Local)
	},
}

// runHistory is the whole verb with its writer, its DIRECTORY and its
// timezone injected. The directory is a parameter for the same reason the
// capture itself is one (see runUpdateWith): a test must never depend on —
// or write to — the operator's real state directory. The location is a
// parameter because rendering a timestamp is otherwise impure, and a test
// that formatted in the developer's zone would pass in one country only.
func runHistory(w io.Writer, args []string, dir string, loc *time.Location) error {
	runs, err := histindex.Scan(dir)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(w, "no update captures yet — a `fleet update` writes one per host per run")
		return nil
	}

	sel := runs
	if len(args) == 1 {
		sel = filterHost(runs, args[0])
		if len(sel) == 0 {
			// Naming the hosts that DO have history turns a typo into a
			// one-line fix, instead of looking exactly like "this host has
			// never been updated".
			fmt.Fprintf(w, "no captures for %q; hosts with history: %s\n",
				args[0], strings.Join(hostsOf(runs), ", "))
			return nil
		}
	}

	if flagHistoryProblems {
		return digestProblems(w, sel, len(args) == 1, loc)
	}
	if flagHistoryShow {
		return showRun(w, sel)
	}
	return listRuns(w, sel, loc)
}

// filterHost keeps one host's runs, matched exactly — a prefix match would
// make "pi" ambiguous the moment a "pi2" joined the fleet.
func filterHost(runs []histindex.Run, host string) []histindex.Run {
	var out []histindex.Run
	for _, r := range runs {
		if r.Host == host {
			out = append(out, r)
		}
	}
	return out
}

// hostsOf lists the distinct hosts with history, alphabetically.
func hostsOf(runs []histindex.Run) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range runs {
		if !seen[r.Host] {
			seen[r.Host] = true
			out = append(out, r.Host)
		}
	}
	sort.Strings(out)
	return out
}

// listRuns renders the table. RESULT reports what the capture actually
// PROVES — whether the run reached its footer — not whether it succeeded: a
// capture records output, not an exit code, and a row claiming "ok" on that
// evidence would be inventing a fact.
func listRuns(w io.Writer, runs []histindex.Run, loc *time.Location) error {
	if flagHistoryLimit > 0 && len(runs) > flagHistoryLimit {
		runs = runs[:flagHistoryLimit]
	}

	sums := make([]histindex.Summary, 0, len(runs))
	for _, r := range runs {
		s, err := histindex.Summarize(r)
		if err != nil {
			// One unreadable file must not cost the whole listing.
			s = histindex.Summary{Run: r}
		}
		sums = append(sums, s)
	}

	if flagJSON {
		return json.NewEncoder(w).Encode(sums)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tWHEN\tHOST\tRESULT\tWARN\tSIZE")
	for i, s := range sums {
		result := "finished"
		if !s.Finished {
			result = "unfinished"
		}
		warn := "-"
		if s.Warnings > 0 {
			warn = fmt.Sprintf("⚠%d", s.Warnings)
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
			i+1, s.At.In(loc).Format("2006-01-02 15:04"), s.Host, result, warn, size(s.Size))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(runs) > 0 {
		fmt.Fprintf(w, "\nread one with: fleet history %s --show --run N\n", runs[0].Host)
	}
	return nil
}

// showRun prints one run's content. --run indexes the SELECTION, so
// `fleet history nano --show --run 2` is nano's second-newest — the number
// means the same thing as the # column just listed.
func showRun(w io.Writer, runs []histindex.Run) error {
	n := flagHistoryRun
	if n <= 0 {
		n = 1
	}
	if n > len(runs) {
		return fmt.Errorf("--run %d: only %d capture(s) available", n, len(runs))
	}
	r := runs[n-1]

	c, err := histindex.Read(r.Path)
	if err != nil {
		return err
	}

	var re *regexp.Regexp
	if flagHistoryGrep != "" {
		if re, err = regexp.Compile(flagHistoryGrep); err != nil {
			return fmt.Errorf("--grep: %w", err)
		}
	}

	lines := c.Lines
	if flagHistoryErrors {
		lines = c.Stderr()
	}

	fmt.Fprintf(w, "# %s\n# %s\n\n", r.Path, c.Header)
	shown := 0
	for _, l := range lines {
		if re != nil && !re.MatchString(l.Text) {
			continue
		}
		// The stderr mark is decoded into a column rather than reprinted:
		// "!! " is the on-disk encoding, not something an operator reads.
		mark := " "
		if l.Stderr {
			mark = "!"
		}
		fmt.Fprintf(w, "%s %s %s\n", l.Time, mark, l.Text)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(w, "(no lines matched)")
	}
	return nil
}

// size renders a byte count compactly.
func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func init() {
	historyCmd.Flags().BoolVar(&flagHistoryShow, "show", false, "print a run's content instead of listing")
	historyCmd.Flags().IntVar(&flagHistoryRun, "run", 1, "which run to show (1 = newest)")
	historyCmd.Flags().BoolVar(&flagHistoryErrors, "errors", false, "with --show, keep only stderr lines")
	historyCmd.Flags().StringVar(&flagHistoryGrep, "grep", "", "with --show, keep only lines matching this regexp")
	historyCmd.Flags().IntVar(&flagHistoryLimit, "limit", historyDefaultLimit, "maximum rows to list (0 = all)")
	historyCmd.Flags().BoolVar(&flagHistoryProblems, "problems", false,
		"digest what went wrong: the installer's own diagnosis first, then non-benign stderr, repeats collapsed (no host = newest run of every host)")
	rootCmd.AddCommand(historyCmd)
}

// digestProblems prints what actually went wrong, deduped.
//
// With no host named it reports each host's NEWEST run only: older runs are
// history, not current state, and a fleet-wide answer to "what is broken"
// must not resurrect a problem that a later run already fixed. With a host
// named it digests that host's selected run (--run, default newest).
//
// A host whose newest run is clean is SAID to be clean rather than omitted.
// Silence is ambiguous — it reads identically to a host that was never
// checked — and the point of the view is to be able to trust it.
func digestProblems(w io.Writer, runs []histindex.Run, oneHost bool, loc *time.Location) error {
	sel := runs
	if !oneHost {
		sel = newestPerHost(runs)
	} else {
		n := flagHistoryRun
		if n <= 0 {
			n = 1
		}
		if n > len(runs) {
			return fmt.Errorf("--run %d: only %d capture(s) available", n, len(runs))
		}
		sel = runs[n-1 : n]
	}

	for _, r := range sel {
		c, err := histindex.Read(r.Path)
		if err != nil {
			fmt.Fprintf(w, "%s  %s  unreadable: %v\n", r.Host, r.At.In(loc).Format("2006-01-02 15:04"), err)
			continue
		}
		ps := c.Problems()
		when := r.At.In(loc).Format("2006-01-02 15:04")
		if len(ps) == 0 {
			// "No problems found" and "I could not see what happened"
			// produce the SAME empty digest. Collapsing them would report a
			// host as verified on the strength of a capture we know is
			// missing the only part that mattered.
			state := "clean"
			if !c.Observed {
				state = "not captured (interactive run — output went to the terminal)"
			}
			fmt.Fprintf(w, "%s  %s  %s\n", r.Host, when, state)
			continue
		}
		fmt.Fprintf(w, "%s  %s  %s\n", r.Host, when, plural(len(ps), "problem"))
		for _, p := range ps {
			// The multiplier replaces the repetition: one failure seen 37
			// times is one line, which is the entire reason this view exists.
			count := ""
			if p.Count > 1 {
				count = fmt.Sprintf("%d× ", p.Count)
			}
			// "!" marks the installer's own diagnosis — the actionable line —
			// apart from the raw stderr underneath it.
			mark := " "
			if p.Authored {
				mark = "!"
			}
			fmt.Fprintf(w, "  %s %s%s\n", mark, count, p.Text)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// newestPerHost keeps the first (newest — Scan sorts descending) run of each
// host, preserving that order.
func newestPerHost(runs []histindex.Run) []histindex.Run {
	seen := map[string]bool{}
	var out []histindex.Run
	for _, r := range runs {
		if seen[r.Host] {
			continue
		}
		seen[r.Host] = true
		out = append(out, r)
	}
	return out
}
