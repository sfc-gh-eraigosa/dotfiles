package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestFeatureLeavesWired pins PR-45: start, the worker group + worker add, and
// list are attached to the right parents.
func TestFeatureLeavesWired(t *testing.T) {
	if !hasSubcommand(featureCmd, "start") {
		t.Error("gss feature start not wired")
	}
	if !hasSubcommand(featureCmd, "worker") {
		t.Error("gss feature worker group not wired")
	}
	if !hasSubcommand(featureCmd, "list") {
		t.Error("gss feature list not wired")
	}
	if !hasSubcommand(featureWorkerCmd, "add") {
		t.Error("gss feature worker add not wired")
	}
}

func TestFeatureStartArgs(t *testing.T) {
	if err := featureStartCmd.Args(featureStartCmd, []string{}); err == nil {
		t.Error("start requires a <name> arg")
	}
	if err := featureStartCmd.Args(featureStartCmd, []string{"auth"}); err != nil {
		t.Errorf("start <name> should accept exactly one arg: %v", err)
	}
	if err := featureStartCmd.Args(featureStartCmd, []string{"a", "b"}); err == nil {
		t.Error("start rejects more than one arg")
	}
}

func TestFeatureWorkerAddRequiredFlags(t *testing.T) {
	for _, name := range []string{"feature", "purpose", "description"} {
		fl := featureWorkerAddCmd.Flags().Lookup(name)
		if fl == nil {
			t.Errorf("worker add missing --%s", name)
			continue
		}
		if fl.Annotations[cobra.BashCompOneRequiredFlag] == nil {
			t.Errorf("worker add --%s should be marked required", name)
		}
	}
}

func TestFeatureListFlags(t *testing.T) {
	for _, name := range []string{"feature", "tree", "json"} {
		if featureListCmd.Flags().Lookup(name) == nil {
			t.Errorf("list missing --%s", name)
		}
	}
}
