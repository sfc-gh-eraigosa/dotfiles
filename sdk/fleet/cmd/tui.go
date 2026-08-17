package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

var (
	tuiUpdateRef string
	tuiJobs      int
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
		raw, err := os.ReadFile(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
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
		m := newTUIModel(selectHosts(all, nil), r, base, time.Now(), tuiUpdateRef, tuiJobs)
		m.wake = newWaker(r, p)
		_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	},
}

func init() {
	tuiCmd.Flags().StringVar(&flagRef, "ref", "origin/main", "baseline git ref (what hosts are compared against)")
	tuiCmd.Flags().StringVar(&tuiUpdateRef, "update-ref", "main", "git ref to update hosts TO")
	tuiCmd.Flags().IntVar(&tuiJobs, "jobs", 4, "max concurrent background updates")
	rootCmd.AddCommand(tuiCmd)
}
