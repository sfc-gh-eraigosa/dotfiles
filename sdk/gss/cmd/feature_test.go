package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// TestFeatureParentRegistered pins the Batch H entry point (PR-44): the
// `feature` parent is attached to root and renders help. Leaves attach in
// PR-45..47 and are asserted there.
func TestFeatureParentRegistered(t *testing.T) {
	if !hasSubcommand(rootCmd, "feature") {
		t.Fatal("gss feature is not registered on rootCmd")
	}
	if featureCmd.Use != "feature" {
		t.Errorf("featureCmd.Use = %q; want feature", featureCmd.Use)
	}
	if featureCmd.Short == "" || featureCmd.Long == "" {
		t.Error("featureCmd should carry Short + Long help")
	}
}

// TestFeatureHelpRenders exercises the acceptance criterion: `gss feature
// --help` produces usage text without error.
func TestFeatureHelpRenders(t *testing.T) {
	var out bytes.Buffer
	featureCmd.SetOut(&out)
	featureCmd.SetArgs([]string{"--help"})
	if err := featureCmd.Help(); err != nil {
		t.Fatalf("feature --help: %v", err)
	}
	if !strings.Contains(out.String(), "feature") {
		t.Errorf("help output missing 'feature':\n%s", out.String())
	}
}
