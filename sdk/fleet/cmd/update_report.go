package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// exitErrorForReports is nil when every host's report is ok, else an error
// naming how many hosts were not updated. Kept separate from runUpdate so it
// is testable without a runner or a plan.
func exitErrorForReports(reports []updexec.HostReport) error {
	var failures int
	for _, r := range reports {
		if r.Failed() {
			failures++
		}
	}
	if failures == 0 {
		return nil
	}
	return fmt.Errorf("%d host(s) not updated", failures)
}

// statusLabel renders a Result's Status per spec F8: ok|FAIL|skip|dep-fail.
func stepStatusLabel(s updexec.Status) string {
	switch s {
	case updexec.OK:
		return "ok"
	case updexec.Failed:
		return "FAIL"
	case updexec.Skipped:
		return "skip"
	case updexec.DepFailed:
		return "dep-fail"
	default:
		return string(s)
	}
}

// printHostReport renders one host's report per spec F8: "=== host ===" then
// one line per step, then "log: <path>" when the run left a capture. p is
// no longer consulted for the retry budget — Result.MaxAttempts carries it
// directly, set by the executor at every return point of runWithRetry (and
// runRestore's fixed policy), so this no longer guesses it from a
// ".restore" id suffix plus a hardcoded 3.
func printHostReport(w io.Writer, p updplan.Plan, rep updexec.HostReport) {
	_ = p // kept for call-site/API stability; no longer used for the retry budget
	fmt.Fprintf(w, "=== %s ===\n", rep.Host)
	for _, res := range rep.Results {
		var parts []string
		parts = append(parts, stepStatusLabel(res.Status), res.Step)
		if res.Status == updexec.OK || res.Status == updexec.Failed {
			parts = append(parts, fmt.Sprintf("[exit %d]", res.Exit))
		}
		if total := res.MaxAttempts; total > 1 || res.Attempts > 1 {
			if total < res.Attempts {
				total = res.Attempts
			}
			parts = append(parts, fmt.Sprintf("[attempt %d/%d]", res.Attempts, total))
		}
		if res.TimedOut {
			parts = append(parts, "[timeout]")
		}
		parts = append(parts, res.Duration.String())
		note := res.Reason
		if note == "" && len(res.Notes) > 0 {
			note = strings.Join(res.Notes, "; ")
		}
		if note != "" {
			parts = append(parts, note)
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}
	if rep.Output != "" {
		fmt.Fprintf(w, "log: %s\n", rep.Output)
	}
}

// printDryRun prints the plan source and, for every step in Order(), the
// exact script(s) that WOULD be sent plus its effective timeout/retry — and
// touches no runner at all, so it is structurally incapable of sending
// anything.
func printDryRun(w io.Writer, p updplan.Plan, local updplan.Local, reset bool) error {
	fmt.Fprintf(w, "plan: %s\n", p.Source)
	// cmd carries no script-selection logic of its own: Executor.Scripts is
	// the SAME builder that runSync/runRun/runGhAuth call, so dry-run can
	// no longer drift from what a live run would actually send (the leaf D
	// review finding: this used to re-implement the per-kind selection and
	// disagreed with it).
	ex := updexec.Executor{Local: local, Reset: reset}
	for _, st := range p.Order() {
		fmt.Fprintf(w, "=== %s (%s) ===\n", st.ID, st.Kind)
		scripts, err := ex.Scripts(p, st)
		if err != nil {
			return err
		}
		for _, ls := range scripts {
			fmt.Fprintf(w, "  %s: %s\n", ls.Label, ls.Script)
		}
		fmt.Fprintf(w, "  timeout=%s retry=%d on=%v\n", st.Timeout, st.Retry.Attempts, st.Retry.On)
	}
	return nil
}

// jsonUpdateOutput is --json's document shape: the plan source plus every
// host's report, verbatim.
type jsonUpdateOutput struct {
	Plan    string               `json:"plan"`
	Reports []updexec.HostReport `json:"reports"`
}

// printJSONReport writes reports as the sole output, nothing else — per
// spec F8, "--json ... nothing else".
func printJSONReport(w io.Writer, p updplan.Plan, reports []updexec.HostReport) error {
	b, err := json.MarshalIndent(jsonUpdateOutput{Plan: p.Source, Reports: reports}, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
