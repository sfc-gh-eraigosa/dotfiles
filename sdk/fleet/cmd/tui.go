package cmd

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

var (
	tuiUpdateRef string
	tuiJobs      int
	tuiFile      string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive host list: vim navigation, search, selection, concurrent updates",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Both flags reach a remote shell / bound concurrency, so they are
		// validated before a single host is contacted.
		if !validRef(tuiUpdateRef) {
			return fmt.Errorf("invalid --update-ref %q: must be a git ref (letters, digits, . _ / -)", tuiUpdateRef)
		}
		if tuiJobs < 1 {
			return fmt.Errorf("invalid --jobs %d: must be at least 1", tuiJobs)
		}
		// The plan is loaded ONCE, here, through the same leaf-D loader the
		// headless `fleet update` uses — the TUI grows no second plan loader.
		// --update-ref is validated against the resolved plan before a single
		// host is contacted, the same way --ref is validated for `update`.
		plan, err := loadPlan(tuiFile, &featflag.GFF{Repo: flagRepo}, flagRepo)
		if err != nil {
			return err
		}
		plan, err = plan.WithRef(tuiUpdateRef)
		if err != nil {
			return fmt.Errorf("invalid --update-ref %q: %w", tuiUpdateRef, err)
		}
		// A MISSING config is an empty fleet, not a failure: on a fresh
		// machine there is nothing to read yet, and refusing to start is
		// how `fleet` became unusable on exactly the host that needed
		// setting up.
		rawStr, err := readConfig(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		raw := []byte(rawStr)
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
		}
		// An empty fleet is where a first run begins, not an error. Offer to
		// set one up before opening a dashboard with nothing in it.
		if len(all) == 0 {
			changed, err := ensureFleet(cmd, len(all))
			if err != nil {
				return err
			}
			if changed {
				rawStr, err := readConfig(flagConfig)
				if err != nil {
					return err
				}
				if all, err = sshconf.Parse(rawStr, flagMarker); err != nil {
					return err
				}
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no hosts to show yet — nothing to display.")
				return nil
			}
		}
		base, err := newGitBaseline(flagRepo, flagRef)
		if err != nil {
			return err
		}
		p, err := wakePolicy()
		if err != nil {
			return err
		}
		// No synchronous probing here: the model opens instantly and streams
		// rows in as each host answers (spec F1).
		r := runner.Exec{}
		m := newTUIModel(selectHosts(all, nil), r, base, time.Now(), tuiUpdateRef, tuiJobs, plan)
		m.wake = newWaker(r, p)
		// Persistence is wired HERE, not in newTUIModel, so the model stays a
		// pure value and tests never touch a real config directory.
		m.ansPath = answersPath()
		m.ans = loadAnswers(m.ansPath)
		// Re-passed to the interactive handoff's self-exec (`fleet update
		// <alias> --file <tuiFile> ...`) so a routed host resolves the SAME
		// plan the TUI itself loaded, not gff's own (possibly different)
		// resolution.
		m.file = tuiFile
		_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	},
}

func init() {
	tuiCmd.Flags().StringVar(&flagRef, "ref", "origin/main", "baseline git ref (what hosts are compared against)")
	tuiCmd.Flags().StringVar(&tuiUpdateRef, "update-ref", "main", "git ref to update hosts TO")
	tuiCmd.Flags().IntVar(&tuiJobs, "jobs", 4, "max concurrent background updates")
	tuiCmd.Flags().StringVar(&tuiFile, "file", "", "explicit fleet.yaml plan path (skips gff resolution)")
	rootCmd.AddCommand(tuiCmd)
}
