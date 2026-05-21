package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/agent"
)

func TestGssFeatureStart_WrapsCall(t *testing.T) {
	var got []string
	run := func(args ...string) ([]byte, error) {
		got = args
		return []byte("Started feature \"auth\"\n"), nil
	}
	var out bytes.Buffer
	if err := gssFeatureStart(run, &out, "auth", "develop", "login work"); err != nil {
		t.Fatalf("gssFeatureStart: %v", err)
	}
	want := "feature start auth --base develop --description login work"
	if strings.Join(got, " ") != want {
		t.Errorf("args = %q; want %q", strings.Join(got, " "), want)
	}
	if !strings.Contains(out.String(), "Started feature") {
		t.Errorf("gss output not forwarded: %q", out.String())
	}
}

func TestGssFeatureStart_MinimalArgs(t *testing.T) {
	var got []string
	run := func(args ...string) ([]byte, error) { got = args; return nil, nil }
	_ = gssFeatureStart(run, &bytes.Buffer{}, "auth", "", "")
	if strings.Join(got, " ") != "feature start auth" {
		t.Errorf("args = %q; want `feature start auth` (no empty flags)", strings.Join(got, " "))
	}
}

func TestFeatureStatus_AnnotatesPaneLiveness(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	// Seed tmux-mgr sessions: one in the feature (alive), one in another feature.
	for _, s := range []agent.Session{
		{SessionID: "s1", WorkerRef: "auth/erai/api", PaneID: "%alive", Status: agent.StatusRunning},
		{SessionID: "s2", WorkerRef: "billing/erai/api", PaneID: "%dead", Status: agent.StatusRunning},
	} {
		if err := agent.SaveSession(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	run := func(args ...string) ([]byte, error) { return []byte("(gss list output)\n"), nil }
	isAlive := func(p string) bool { return p == "%alive" }

	out, err := featureStatus(run, isAlive, "auth")
	if err != nil {
		t.Fatalf("featureStatus: %v", err)
	}
	if !strings.Contains(out, "auth/erai/api  pane=%alive (alive)") {
		t.Errorf("missing live auth pane annotation:\n%s", out)
	}
	if strings.Contains(out, "billing/erai/api") {
		t.Errorf("feature filter leaked a billing worker:\n%s", out)
	}
}

func TestFeatureAddAgent_RequiresFlags(t *testing.T) {
	// Validation short-circuits before any exec/tmux.
	addAgentPurpose, addAgentTask = "", ""
	if err := featAddAgentCmd.RunE(featAddAgentCmd, []string{"auth"}); err == nil {
		t.Error("add-agent without --purpose should error")
	}
	addAgentPurpose = "api"
	if err := featAddAgentCmd.RunE(featAddAgentCmd, []string{"auth"}); err == nil {
		t.Error("add-agent without --task-description should error")
	}
	addAgentPurpose, addAgentTask = "", "" // reset
}

func TestFeatureVerbsWired(t *testing.T) {
	if !hasSub(rootCmd, "feature") {
		t.Fatal("tmux-mgr feature not registered")
	}
	for _, name := range []string{"start", "add-agent", "status"} {
		if !hasSub(tmuxFeatureCmd, name) {
			t.Errorf("feature %s not wired", name)
		}
	}
}
