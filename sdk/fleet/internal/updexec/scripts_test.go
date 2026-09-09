package updexec

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// TestScriptsListsPrecheckSyncAndConditionalsForASyncStep pins Executor.Scripts'
// output for a sync step: precheck and sync are always present, and clone/
// rescue are present but labelled conditional — Scripts cannot know the
// live remote state, so it lists everything that MIGHT be sent rather than
// guessing.
func TestScriptsListsPrecheckSyncAndConditionalsForASyncStep(t *testing.T) {
	plan := onlySyncPlan(updplan.LocalRescue, true)
	st, _ := plan.Step("dotfiles.sync")
	e := Executor{Local: updplan.LocalRescue}

	scripts, err := e.Scripts(plan, st)
	if err != nil {
		t.Fatal(err)
	}

	var labels []string
	for _, ls := range scripts {
		labels = append(labels, ls.Label)
	}
	joined := strings.Join(labels, "|")
	for _, want := range []string{"precheck", "clone (if missing)", "rescue (if dirty)", "sync (local=rescue)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Scripts() labels = %v, want to contain %q", labels, want)
		}
	}

	for _, ls := range scripts {
		switch ls.Label {
		case "clone (if missing)", "rescue (if dirty)":
			if !ls.Optional {
				t.Fatalf("%q must be marked conditional (state-dependent)", ls.Label)
			}
		case "precheck", "sync (local=rescue)":
			if ls.Optional {
				t.Fatalf("%q is unconditional and must not be marked Optional", ls.Label)
			}
		}
	}
}

// TestScriptsLabelsAreNeverEmptyAndCarryNoRawInjection is a property test:
// for every step kind, every returned label is non-empty, and a
// maliciously-crafted, unvalidated repo/step never produces a script at
// all — Scripts' builder calls (PrecheckScript, SyncScript, RunScript, …)
// re-validate their inputs and return an error rather than silently
// embedding an unvalidated field (e.g. a semicolon-laden path) into a
// script string.
func TestScriptsLabelsAreNeverEmptyAndCarryNoRawInjection(t *testing.T) {
	e := Executor{}

	cases := []struct {
		name string
		plan updplan.Plan
		st   updplan.Step
	}{
		{"sync", onlySyncPlan(updplan.LocalSkip, false), mustStep(t, onlySyncPlan(updplan.LocalSkip, false), "dotfiles.sync")},
		{"run", updplan.Plan{}, updplan.Step{ID: "run1", Kind: updplan.KindRun, Run: "./install.sh"}},
		{"gh-auth", updplan.Plan{}, updplan.Step{ID: "gh1", Kind: updplan.KindGhAuth}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scripts, err := e.Scripts(c.plan, c.st)
			if err != nil {
				t.Fatal(err)
			}
			if len(scripts) == 0 {
				t.Fatal("expected at least one script")
			}
			for _, ls := range scripts {
				if ls.Label == "" {
					t.Fatalf("empty label for script %q", ls.Script)
				}
			}
		})
	}

	// A repo whose Path carries a shell metacharacter must be REJECTED, not
	// silently embedded into the sync script.
	injected := updplan.Plan{
		Repos: map[string]updplan.Repo{
			"evil": {Name: "evil", Path: "~/git/evil; rm -rf /", Branches: []string{"main"}},
		},
		Steps: []updplan.Step{{ID: "evil.sync", Kind: updplan.KindSync, Repo: "evil"}},
	}
	st, _ := injected.Step("evil.sync")
	if _, err := e.Scripts(injected, st); err == nil {
		t.Fatal("a repo path containing a shell metacharacter must be rejected, not embedded into a script")
	}
}

func mustStep(t *testing.T, p updplan.Plan, id string) updplan.Step {
	t.Helper()
	st, ok := p.Step(id)
	if !ok {
		t.Fatalf("no step %q in plan", id)
	}
	return st
}
