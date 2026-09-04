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

// totalAttemptsFor is the retry budget a step's report line compares its
// Attempts against. The synthesized "<repo>.restore" step is not itself in
// the plan, so its fixed policy (3 attempts) is hardcoded here to match
// exec.go's runRestore.
func totalAttemptsFor(p updplan.Plan, stepID string) int {
	if strings.HasSuffix(stepID, ".restore") {
		return 3
	}
	if st, ok := p.Step(stepID); ok {
		if st.Retry.Attempts >= 1 {
			return st.Retry.Attempts
		}
	}
	return 1
}

// printHostReport renders one host's report per spec F8: "=== host ===" then
// one line per step, then "log: <path>" when the run left a capture.
func printHostReport(w io.Writer, p updplan.Plan, rep updexec.HostReport) {
	fmt.Fprintf(w, "=== %s ===\n", rep.Host)
	for _, res := range rep.Results {
		var parts []string
		parts = append(parts, stepStatusLabel(res.Status), res.Step)
		if res.Status == updexec.OK || res.Status == updexec.Failed {
			parts = append(parts, fmt.Sprintf("[exit %d]", res.Exit))
		}
		if total := totalAttemptsFor(p, res.Step); total > 1 || res.Attempts > 1 {
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
	for _, st := range p.Order() {
		fmt.Fprintf(w, "=== %s (%s) ===\n", st.ID, st.Kind)
		switch st.Kind {
		case updplan.KindSync:
			repo, ok := p.RepoOf(st)
			if !ok {
				return fmt.Errorf("dry-run: step %q: unknown repo %q", st.ID, st.Repo)
			}
			eff := local
			if eff == "" {
				eff = repo.Local
			}
			pc, err := updexec.PrecheckScript(repo)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  precheck: %s\n", pc)
			if repo.URL != "" {
				cs, err := updexec.CloneScript(repo)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "  clone (if missing): %s\n", cs)
			}
			if eff == updplan.LocalRescue {
				rs, err := updexec.RescueScript(repo)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "  rescue (if dirty): %s\n", rs)
			}
			ss, err := updexec.SyncScript(repo, eff, reset)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  sync (local=%s): %s\n", eff, ss)
		case updplan.KindRun:
			var repoPtr *updplan.Repo
			if repo, ok := p.RepoOf(st); ok {
				repoPtr = &repo
			}
			rs, err := updexec.RunScript(st, repoPtr)
			if err != nil {
				return err
			}
			label := "run"
			if st.Interactive {
				label = "run (interactive)"
			}
			fmt.Fprintf(w, "  %s: %s\n", label, rs)
		case updplan.KindGhAuth:
			cs, err := updexec.GhAuthCheck(st.Hostname)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  check: %s\n", cs)
			ls, err := updexec.GhAuthLogin(st.Hostname)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  login (if check fails, interactive): %s\n", ls)
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
