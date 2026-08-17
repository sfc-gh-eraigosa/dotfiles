package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

type model struct {
	rows   []Row
	cursor int
	now    time.Time
	status string
}

func newModel(rows []Row, now time.Time) model {
	sortWorstFirst(rows)
	return model{rows: rows, now: now}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) moveCursor(d int) model {
	if len(m.rows) == 0 {
		m.cursor = 0
		return m
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	return m
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString("fleet — u: update  r: refresh  q: quit\n\n")
	if len(m.rows) == 0 {
		b.WriteString("  no fleet hosts found\n")
		b.WriteString("  run `fleet discover` to see adoptable ssh-config hosts, then `fleet add <alias>`\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  %-16s %-9s %-13s %s\n", "HOST", "COMMIT", "LAST RUN", "STATUS")
	for i, r := range m.rows {
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		commit := r.Commit
		if commit == "" {
			commit = "-"
		}
		fmt.Fprintf(&b, "%s%-16s %-9s %-13s %s\n", cur, r.Alias, commit,
			drift0(m.now, r), statusLabel(r))
	}
	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}
	return b.String()
}

// interactiveUpdate builds the command tea.Exec runs with the terminal handed
// over, so install.sh's sudo prompt reaches the operator. It reuses
// remoteUpdateScript from the headless path — one definition of "update a
// host" — targeting main (the TUI's --ref is the drift baseline, not the
// update target).
func interactiveUpdate(host string) *exec.Cmd {
	return exec.Command("ssh", "-t", host, remoteUpdateScript("main"))
}

type refreshMsg struct{}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.status = "refresh: re-run `fleet status` to pick up the new stamp"
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			return m.moveCursor(-1), nil
		case "down", "j":
			return m.moveCursor(1), nil
		case "u":
			if len(m.rows) == 0 {
				return m, nil
			}
			host := m.rows[m.cursor].Alias
			// Release the terminal: install.sh prompts for sudo, so its I/O
			// must NOT be captured by the TUI.
			return m, tea.ExecProcess(interactiveUpdate(host), func(error) tea.Msg { return refreshMsg{} })
		}
	}
	return m, nil
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive host list with an update action",
	RunE: func(cmd *cobra.Command, _ []string) error {
		raw, err := os.ReadFile(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
		}
		hosts := selectHosts(all, nil)
		base, err := newGitBaseline(flagRepo, flagRef)
		if err != nil {
			return err
		}
		now := time.Now()
		rows := collect(hosts, runner.Exec{}, base, now)
		_, err = tea.NewProgram(newModel(rows, now)).Run()
		return err
	},
}

func init() {
	tuiCmd.Flags().StringVar(&flagRef, "ref", "origin/main", "baseline git ref")
	rootCmd.AddCommand(tuiCmd)
}
