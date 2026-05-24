package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestRootCommandTree pins the full gss command surface (PR-48): the feature
// subtree plus every classic verb is registered on root. The flat cmd/ layout
// (cmd-leaf decision) registers each via its own init(); this is the
// exhaustive `gss --help` assertion.
func TestRootCommandTree(t *testing.T) {
	for _, name := range []string{
		"feature", "push", "pr", "sync", "status", "scan", "backup", "config", "version", "diff",
	} {
		if !hasSubcommand(rootCmd, name) {
			t.Errorf("gss %s is not registered on root", name)
		}
	}
}

// TestConfigSubtree pins `gss config print|check` (PR-05 wiring, re-asserted
// as part of the PR-48 surface check).
func TestConfigSubtree(t *testing.T) {
	var configCmd *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "config" {
			configCmd = c
			break
		}
	}
	if configCmd == nil {
		t.Fatal("gss config not registered")
	}
	for _, sub := range []string{"print", "check"} {
		if !hasSubcommand(configCmd, sub) {
			t.Errorf("gss config %s not wired", sub)
		}
	}
}
