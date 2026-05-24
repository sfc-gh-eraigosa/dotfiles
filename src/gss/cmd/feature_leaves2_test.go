package cmd

import "testing"

// TestFeatureLeaves2Wired pins PR-46: checkpoint, conflicts, pr, rebase, and
// restack are attached to featureCmd.
func TestFeatureLeaves2Wired(t *testing.T) {
	for _, name := range []string{"checkpoint", "conflicts", "pr", "rebase", "restack"} {
		if !hasSubcommand(featureCmd, name) {
			t.Errorf("gss feature %s not wired", name)
		}
	}
}

func TestFeatureCheckpointFlags(t *testing.T) {
	for _, name := range []string{"worker", "auto", "dry-run"} {
		if featureCheckpointCmd.Flags().Lookup(name) == nil {
			t.Errorf("checkpoint missing --%s", name)
		}
	}
}

func TestFeaturePRFlags(t *testing.T) {
	for _, name := range []string{"ready", "force", "worker"} {
		if featurePRCmd.Flags().Lookup(name) == nil {
			t.Errorf("pr missing --%s", name)
		}
	}
}

func TestFeatureRestackArgsAndOnto(t *testing.T) {
	if err := featureRestackCmd.Args(featureRestackCmd, []string{}); err == nil {
		t.Error("restack requires a <worker> arg")
	}
	if err := featureRestackCmd.Args(featureRestackCmd, []string{"auth/erai/api"}); err != nil {
		t.Errorf("restack <worker> should accept one arg: %v", err)
	}
	onto := featureRestackCmd.Flags().Lookup("onto")
	if onto == nil {
		t.Fatal("restack missing --onto")
	}
}

func TestFeatureConflictsFlags(t *testing.T) {
	for _, name := range []string{"feature", "json"} {
		if featureConflictsCmd.Flags().Lookup(name) == nil {
			t.Errorf("conflicts missing --%s", name)
		}
	}
}
