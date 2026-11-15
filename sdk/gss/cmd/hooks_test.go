package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestHooksCheckWired pins `gss hooks check` and its --fix flag.
func TestHooksCheckWired(t *testing.T) {
	if !hasSubcommand(hooksCmd, "check") {
		t.Fatal("gss hooks check is not wired")
	}
	if hooksCheckCmd.Flags().Lookup("fix") == nil {
		t.Fatal("gss hooks check has no --fix flag")
	}
}

// TestPushPathsUseHookGuard pins the wiring that makes every push gss makes
// refuse a repo whose own core.hooksPath switches off the global privacy
// hooks: each command file that builds a pushing service wraps its git
// runner and its approval gate in hookguard. Behaviour is tested in
// internal/hookguard; this catches a future edit that drops the wrapper.
func TestPushPathsUseHookGuard(t *testing.T) {
	for _, f := range []string{"push.go", "pr.go", "feature_service.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"hookguard.Runner{Inner: sys}", "hookguard.Approver{"} {
			if !strings.Contains(string(src), want) {
				t.Errorf("cmd/%s does not contain %q", f, want)
			}
		}
	}
}
