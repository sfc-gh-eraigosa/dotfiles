package cmd

import (
	"os"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
	applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"
)

// captureOutput is the CLI's updexec.Output: every host's run is teed to a
// per-run capture under dir (fleetLogDir() in production), through the same
// libs/log lifecycle every other fleet capture uses. A capture that cannot
// be opened must never cost the update — Open falls back to updexec.Discard
// in that case, exactly like a nil Executor.Out would.
type captureOutput struct{ dir string }

func (c captureOutput) Open(host, header string) (updexec.LineWriter, string) {
	cap := applog.NewCapture(applog.CaptureOptions{
		Tool:    logTool,
		Dir:     c.dir,
		Subject: host,
		Header:  header,
		Now:     nowFn,
	})
	if cap == nil {
		return updexec.Discard{}.Open(host, header)
	}
	return captureLineWriter{cap}, cap.Path()
}

// captureLineWriter adapts *applog.Capture to updexec.LineWriter.
type captureLineWriter struct{ c *applog.Capture }

func (w captureLineWriter) Line(s string)       { w.c.WriteLine(s) }
func (w captureLineWriter) Close(footer string) { _ = w.c.Close(footer) }

// newRunLogOutput is the Output every live (non-dry-run) host runs through.
// host is unused today (the capture is keyed by Subject inside Open, not at
// construction) but kept as a parameter so a future per-host directory
// override has somewhere to land.
func newRunLogOutput(string) updexec.Output {
	return captureOutput{dir: fleetLogDir()}
}

// localAnswerPreamble exports the operator's pre-supplied install.sh answers
// — read from the LOCAL environment, never a stored credential — as shell
// assignments prefixed onto a run step's script. Console.runScript already
// gates this to updplan.KindRun steps only; the CLI lane never has a sudo
// secret to send at all (Console.Stdin is left nil), so there is nothing
// else for this preamble to carry.
func localAnswerPreamble(updplan.Step) string {
	var assigns []string
	if v := os.Getenv("WINSETUP_ANSWER"); v != "" {
		assigns = append(assigns, "WINSETUP_ANSWER="+v)
	}
	if v := os.Getenv("GEMINI_TEARDOWN_ANSWER"); v != "" {
		assigns = append(assigns, "GEMINI_TEARDOWN_ANSWER="+v)
	}
	if len(assigns) == 0 {
		return ""
	}
	// No trailing punctuation: Console.runScript joins this with " && " onto
	// the step's script, and "export A=1; && cmd" is a shell syntax error —
	// `&&` cannot directly follow a bare `;`.
	return "export " + strings.Join(assigns, " ")
}
