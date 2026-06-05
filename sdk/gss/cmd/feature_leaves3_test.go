package cmd

import "testing"

// TestFeatureLeaves3Wired pins PR-47: done, merged, and audit are attached to
// featureCmd — completing the gss feature verb surface.
func TestFeatureLeaves3Wired(t *testing.T) {
	for _, name := range []string{"done", "merged", "audit"} {
		if !hasSubcommand(featureCmd, name) {
			t.Errorf("gss feature %s not wired", name)
		}
	}
}

func TestFeatureDoneMergedArgs(t *testing.T) {
	// Both accept zero (cwd-resolved) or one positional worker-ref.
	for _, c := range []*struct {
		name string
		args func([]string) error
	}{
		{"done", func(a []string) error { return featureDoneCmd.Args(featureDoneCmd, a) }},
		{"merged", func(a []string) error { return featureMergedCmd.Args(featureMergedCmd, a) }},
	} {
		if err := c.args([]string{}); err != nil {
			t.Errorf("%s should accept zero args (cwd-resolved): %v", c.name, err)
		}
		if err := c.args([]string{"auth/erai/api"}); err != nil {
			t.Errorf("%s should accept one arg: %v", c.name, err)
		}
		if err := c.args([]string{"a", "b"}); err == nil {
			t.Errorf("%s should reject two args", c.name)
		}
	}
}

func TestFeatureAuditFlags(t *testing.T) {
	for _, name := range []string{"feature", "repair", "json"} {
		if featureAuditCmd.Flags().Lookup(name) == nil {
			t.Errorf("audit missing --%s", name)
		}
	}
}

func TestFeatureMergedFlag(t *testing.T) {
	if featureMergedCmd.Flags().Lookup("no-auto-ready") == nil {
		t.Error("merged missing --no-auto-ready")
	}
	if featureDoneCmd.Flags().Lookup("force") == nil {
		t.Error("done missing --force")
	}
}
