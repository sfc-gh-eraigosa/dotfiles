package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
)

func TestGssWorkerAdd_FlagsAndParse(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(args) >= 3 && args[1] == "worker" && args[2] == "add" {
			return []byte(`{"worker_ref":"auth/erai/api","branch":"feature/auth/erai/api","worktree_path":"/wt/api","base_branch":"main"}`), nil
		}
		return nil, nil // feature start
	}
	res, err := gssWorkerAdd(run, "auth", "api", "do the api", "erai", "claude", "sess-1", "tmgr-9")
	if err != nil {
		t.Fatalf("gssWorkerAdd: %v", err)
	}
	if res.WorkerRef != "auth/erai/api" || res.WorktreePath != "/wt/api" ||
		res.Branch != "feature/auth/erai/api" || res.BaseBranch != "main" {
		t.Errorf("parsed result wrong: %+v", res)
	}
	// First call ensures the feature exists.
	if len(calls) != 2 || calls[0][0] != "feature" || calls[0][1] != "start" || calls[0][2] != "auth" {
		t.Fatalf("expected `feature start auth` first; got %v", calls)
	}
	// Second call carries all the required flags.
	got := strings.Join(calls[1], " ")
	for _, want := range []string{
		"feature worker add", "--feature auth", "--purpose api",
		"--description do the api", "--user erai", "--engine claude",
		"--session-id sess-1", "--tmux-mgr-session tmgr-9", "--json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("worker add args missing %q; got %q", want, got)
		}
	}
}

func TestGssWorkerAdd_ErrorOnBadJSON(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "add" {
			return []byte("not json"), nil
		}
		return nil, nil
	}
	if _, err := gssWorkerAdd(run, "f", "p", "d", "u", "claude", "", "t"); err == nil {
		t.Error("want error on non-JSON worker add output")
	}
}

func TestGssWorkerAdd_ErrorPropagated(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "add" {
			return []byte("boom"), errors.New("exit 1")
		}
		return nil, nil
	}
	if _, err := gssWorkerAdd(run, "f", "p", "d", "u", "claude", "", "t"); err == nil {
		t.Error("want error when gss worker add fails")
	}
}

func TestEngineSessionID(t *testing.T) {
	env := map[string]string{"CLAUDE_SESSION_ID": "c1", "ANTIGRAVITY_SESSION_ID": "a1", "GEMINI_SESSION_ID": "g1"}
	get := func(k string) string { return env[k] }
	if got := engineSessionID(agent.AssistantClaude, get); got != "c1" {
		t.Errorf("claude session = %q; want c1", got)
	}
	if got := engineSessionID(agent.AssistantAntigravity, get); got != "a1" {
		t.Errorf("antigravity session = %q; want a1", got)
	}
	// A legacy "gemini" host prefers its own GEMINI_SESSION_ID over the
	// Antigravity var when both are set.
	if got := engineSessionID(agent.AssistantGemini, get); got != "g1" {
		t.Errorf("legacy gemini session = %q; want g1", got)
	}

	// Without the new var, the legacy GEMINI_SESSION_ID is still honored.
	legacyEnv := map[string]string{"GEMINI_SESSION_ID": "g1"}
	getLegacy := func(k string) string { return legacyEnv[k] }
	if got := engineSessionID(agent.AssistantAntigravity, getLegacy); got != "g1" {
		t.Errorf("antigravity legacy fallback session = %q; want g1", got)
	}
	// And the reverse: a legacy gemini host falls back to the Antigravity var.
	agyEnv := map[string]string{"ANTIGRAVITY_SESSION_ID": "a1"}
	getAgy := func(k string) string { return agyEnv[k] }
	if got := engineSessionID(agent.AssistantGemini, getAgy); got != "a1" {
		t.Errorf("gemini fallback to antigravity session = %q; want a1", got)
	}
}

func TestWrapWithPaneWrap(t *testing.T) {
	got := wrapWithPaneWrap("/usr/bin/tmux-mgr", "auth/erai/api", "ENV=x agent execute -d 'task'")
	for _, want := range []string{
		"/usr/bin/tmux-mgr internal pane-wrap",
		"--worker-ref 'auth/erai/api'",
		"-- /bin/sh -c '",
		"agent execute",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wrapped cmd missing %q; got %q", want, got)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf(`shellQuote(a'b) = %s; want 'a'\''b'`, got)
	}
}

func TestCleanupSession_WorkerRef(t *testing.T) {
	for _, force := range []bool{false, true} {
		var gotArgs []string
		var killed string
		var out bytes.Buffer
		s := &agent.Session{SessionID: "s1", WorkerRef: "auth/erai/api", PaneID: "%7", WorktreePath: "/wt/api"}
		cleanupSession(s, force, cleanupDeps{
			run:      func(args ...string) ([]byte, error) { gotArgs = args; return nil, nil },
			killPane: func(p string) error { killed = p; return nil },
			out:      &out,
		})
		want := "feature done --worker auth/erai/api"
		got := strings.Join(gotArgs, " ")
		if force {
			want += " --force"
		}
		if got != want {
			t.Errorf("force=%v: gss args = %q; want %q", force, got, want)
		}
		if killed != "%7" {
			t.Errorf("pane not killed: %q", killed)
		}
	}
}

// TestCleanupSession_LegacyNoWorkerRef pins the PR-59 behaviour: tmux-mgr no
// longer removes worktrees directly (gss owns teardown; currentRepoRoot + the
// direct `git worktree remove` are gone). A no-WorkerRef session just kills the
// pane and is told to migrate; its worktree is left in place.
func TestCleanupSession_LegacyNoWorkerRef(t *testing.T) {
	runCalled := false
	killed := ""
	var out bytes.Buffer
	s := &agent.Session{SessionID: "old", WorkerRef: "", PaneID: "%9", WorktreePath: "/wt/old"}
	cleanupSession(s, false, cleanupDeps{
		run:      func(args ...string) ([]byte, error) { runCalled = true; return nil, nil },
		killPane: func(p string) error { killed = p; return nil },
		out:      &out,
	})
	if runCalled {
		t.Error("legacy session (no WorkerRef) must NOT call gss feature done")
	}
	if killed != "%9" {
		t.Errorf("pane not killed: %q", killed)
	}
	if !strings.Contains(out.String(), "migrate-to-gss") {
		t.Errorf("legacy cleanup should point at migrate-to-gss; got %q", out.String())
	}
	if !strings.Contains(out.String(), "/wt/old") {
		t.Errorf("legacy cleanup should name the retained worktree; got %q", out.String())
	}
}
