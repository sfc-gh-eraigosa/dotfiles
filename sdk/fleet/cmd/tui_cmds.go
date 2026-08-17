package cmd

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	spinnerTickMsg int
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinnerTick(n int) tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg(n) })
}

// pollHost probes one host off the UI thread and streams the row back (F1).
func pollHost(h sshconf.Host, r runner.Runner, base Baseliner) tea.Cmd {
	return func() tea.Msg { return hostRowMsg{row: probeHost(h, r, base)} }
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

// batchModeUpdate wraps the shared update script so it can never block on a
// prompt it has no terminal to answer. BatchMode makes ssh fail fast instead
// of hanging invisibly, so an unexpected password request surfaces as a
// visible FAIL with its cause in the row's log.
func batchModeUpdate(ref string) string {
	return fmt.Sprintf("BatchMode=yes %s", remoteUpdateScript(ref))
}

// bgUpdate runs an update WITHOUT taking the terminal, so many hosts update
// at once and the TUI stays interactive. This is the default lane; the
// ExecProcess handoff below is reserved for hosts that need to prompt.
func bgUpdate(alias, ref string, r runner.Runner) tea.Cmd {
	return func() tea.Msg {
		out, err := r.Run(alias, batchModeUpdate(ref))
		return bgUpdateDoneMsg{alias: alias, log: tailLines(out, 3), err: err}
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
