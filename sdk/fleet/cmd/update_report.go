package cmd

import (
	"io"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// localAnswerPreamble and newRunLogOutput are filled in by task 23 (the
// headless run log + answers env). Until then a run carries no capture and
// no preamble.
func localAnswerPreamble(updplan.Step) string { return "" }

func newRunLogOutput(string) updexec.Output { return nil }

// printHostReport, printDryRun and printJSONReport are filled in by task 21
// (per-step report, --json, --dry-run). Task 20 only needs them to exist so
// runUpdate compiles; the CLI's report rendering is not yet under test.
func printHostReport(w io.Writer, _ updplan.Plan, rep updexec.HostReport) {
	_, _ = w, rep
}

func printDryRun(w io.Writer, p updplan.Plan, _ updplan.Local, _ bool) error {
	_, _ = w, p
	return nil
}

func printJSONReport(w io.Writer, p updplan.Plan, reports []updexec.HostReport) error {
	_, _, _ = w, p, reports
	return nil
}
