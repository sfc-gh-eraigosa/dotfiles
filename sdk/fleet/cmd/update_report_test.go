package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// TestReportNamesEveryStepAndTheLog asserts the per-host report names every
// step (with an attempt marker when retried and a timeout marker when the
// attempt was cancelled) and, when the run left a capture, a trailing
// "log: <path>" line.
func TestReportNamesEveryStepAndTheLog(t *testing.T) {
	plan := updplan.Plan{
		Source: "test-source",
		Steps: []updplan.Step{
			{ID: "dotfiles.sync", Kind: updplan.KindSync, Retry: updplan.Retry{Attempts: 3}},
			{ID: "dotfiles.install", Kind: updplan.KindRun},
		},
	}
	rep := updexec.HostReport{
		Host: "alpha", Plan: plan.Source, Output: "/tmp/alpha.log",
		Results: []updexec.Result{
			{Step: "dotfiles.sync", Kind: updplan.KindSync, Status: updexec.OK, Exit: 0, Attempts: 2, MaxAttempts: 3, Duration: 3 * time.Second},
			{Step: "dotfiles.install", Kind: updplan.KindRun, Status: updexec.Failed, Exit: 1, Reason: "boom", TimedOut: true, Duration: time.Second},
		},
	}
	var buf strings.Builder
	printHostReport(&buf, plan, rep)
	out := buf.String()
	for _, want := range []string{"=== alpha ===", "dotfiles.sync", "attempt 2/3", "dotfiles.install", "FAIL", "timeout", "boom", "log: /tmp/alpha.log"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

// A skipped step must never claim an exit code, and a report with no
// capture must have no "log:" line at all.
func TestReportOmitsExitOnSkipAndOmitsLogWhenThereIsNone(t *testing.T) {
	plan := updplan.Plan{Steps: []updplan.Step{{ID: "s", Kind: updplan.KindSync}}}
	rep := updexec.HostReport{
		Host: "alpha",
		Results: []updexec.Result{
			{Step: "s", Kind: updplan.KindSync, Status: updexec.Skipped, Reason: "clone is dirty"},
		},
	}
	var buf strings.Builder
	printHostReport(&buf, plan, rep)
	out := buf.String()
	if strings.Contains(out, "[exit") {
		t.Fatalf("a skipped step must not claim an exit code:\n%s", out)
	}
	if strings.Contains(out, "log:") {
		t.Fatalf("an empty capture path must produce no log line:\n%s", out)
	}
}

// TestExitCodeReflectsAnyFailedHost asserts the CLI's exit-triggering error
// names exactly how many hosts failed, and is nil when every host is ok.
func TestExitCodeReflectsAnyFailedHost(t *testing.T) {
	ok := updexec.HostReport{Results: []updexec.Result{{Step: "s", Status: updexec.OK}}}
	bad := updexec.HostReport{Results: []updexec.Result{{Step: "s", Status: updexec.Failed}}}

	if err := exitErrorForReports([]updexec.HostReport{ok}); err != nil {
		t.Fatalf("all-ok reports must yield a nil error, got %v", err)
	}
	err := exitErrorForReports([]updexec.HostReport{ok, bad})
	if err == nil || !strings.Contains(err.Error(), "1 host(s) not updated") {
		t.Fatalf("got %v, want an error naming 1 failed host", err)
	}
	err = exitErrorForReports([]updexec.HostReport{bad, bad})
	if err == nil || !strings.Contains(err.Error(), "2 host(s) not updated") {
		t.Fatalf("got %v, want an error naming 2 failed hosts", err)
	}
}

// TestDryRunSendsNothing asserts --dry-run prints the plan source, every
// effective script (precheck + sync for the default plan's sync step, the
// verbatim run script for its install step), and each step's effective
// timeout/retry — all without ever being handed a runner, so it is
// structurally incapable of sending anything.
func TestDryRunSendsNothing(t *testing.T) {
	plan := updplan.Default()
	var buf strings.Builder
	if err := printDryRun(&buf, plan, "", false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"plan: " + plan.Source,
		"git fetch origin main",
		"git checkout main",
		"merge --ff-only FETCH_HEAD",
		"./install.sh",
		"timeout=",
		"retry=",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry run missing %q:\n%s", want, out)
		}
	}
}

// TestJSONReportIsMachineReadable asserts --json emits a document that
// round-trips through encoding/json and carries the plan source plus every
// host's report.
func TestJSONReportIsMachineReadable(t *testing.T) {
	plan := updplan.Plan{Source: "src"}
	reports := []updexec.HostReport{
		{Host: "a", Results: []updexec.Result{{Step: "s", Kind: updplan.KindSync, Status: updexec.OK}}},
	}
	var buf strings.Builder
	if err := printJSONReport(&buf, plan, reports); err != nil {
		t.Fatal(err)
	}
	var out jsonUpdateOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if out.Plan != "src" || len(out.Reports) != 1 || out.Reports[0].Host != "a" {
		t.Fatalf("got %+v", out)
	}
}

// TestReportNeverShowsAttemptOneOfOneForSingleAttemptSteps pins that
// Result.MaxAttempts (set by the executor at every runWithRetry return
// point, replacing the old totalAttemptsFor id-suffix guess) suppresses the
// "[attempt N/M]" marker entirely for a step that only ever gets one
// attempt — a "1/1" marker is noise, not information.
func TestReportNeverShowsAttemptOneOfOneForSingleAttemptSteps(t *testing.T) {
	plan := updplan.Plan{Steps: []updplan.Step{{ID: "s", Kind: updplan.KindRun}}}
	rep := updexec.HostReport{
		Host: "alpha",
		Results: []updexec.Result{
			{Step: "s", Kind: updplan.KindRun, Status: updexec.OK, Exit: 0, Attempts: 1, MaxAttempts: 1},
		},
	}
	var buf strings.Builder
	printHostReport(&buf, plan, rep)
	if strings.Contains(buf.String(), "attempt") {
		t.Fatalf("a single-attempt step must never show an attempt marker:\n%s", buf.String())
	}
}

// TestReportShowsAttemptForARestoreStep pins that a synthesized
// "<repo>.restore" step's report line reads "attempt N/3" straight from
// Result.MaxAttempts, without cmd guessing the budget from the ".restore"
// id suffix (the deleted totalAttemptsFor).
func TestReportShowsAttemptForARestoreStep(t *testing.T) {
	plan := updplan.Plan{} // the restore step is synthesized, not in the plan
	rep := updexec.HostReport{
		Host: "alpha",
		Results: []updexec.Result{
			{Step: "dotfiles.restore", Kind: updplan.KindSync, Status: updexec.OK, Exit: 0, Attempts: 2, MaxAttempts: 3},
		},
	}
	var buf strings.Builder
	printHostReport(&buf, plan, rep)
	if !strings.Contains(buf.String(), "attempt 2/3") {
		t.Fatalf("restore step report missing %q:\n%s", "attempt 2/3", buf.String())
	}
}
