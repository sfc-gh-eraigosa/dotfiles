package cmd

import (
	"encoding/json"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

func TestBuildSpawnedBy(t *testing.T) {
	if sb := buildSpawnedBy("", "", "", "", "2026-05-21T00:00:00Z"); sb != nil {
		t.Errorf("no provenance flags → want nil; got %+v", sb)
	}
	sb := buildSpawnedBy("claude", "sess1", "%3", "team-a", "2026-05-21T00:00:00Z")
	if sb == nil {
		t.Fatal("with provenance flags → want non-nil")
	}
	if sb.Engine != "claude" || sb.SessionID != "sess1" || sb.PaneID != "%3" ||
		sb.TmuxMgrSession != "team-a" || sb.StartedAt != "2026-05-21T00:00:00Z" {
		t.Errorf("spawned_by not populated correctly: %+v", sb)
	}
	if sb := buildSpawnedBy("antigravity", "sess2", "", "", "2026-05-21T00:00:00Z"); sb == nil || sb.Engine != "antigravity" {
		t.Errorf("antigravity engine → want antigravity; got %+v", sb)
	}
	// The legacy Gemini CLI engine value is accepted but normalized so new
	// registry rows carry the current identifier.
	if sb := buildSpawnedBy("gemini", "sess3", "", "", "2026-05-21T00:00:00Z"); sb == nil || sb.Engine != "antigravity" {
		t.Errorf("legacy gemini engine → want normalized to antigravity; got %+v", sb)
	}
}

func TestWorkerAddJSON(t *testing.T) {
	res := feature.WorkerResult{
		Ref:      identity.WorkerRef{Feature: "auth", User: "erai", Purpose: "api"},
		Branch:   "feature/auth/erai/api",
		Worktree: "/wt/api",
		Base:     "main",
	}
	data, err := workerAddJSON(res)
	if err != nil {
		t.Fatalf("workerAddJSON: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}
	want := map[string]string{
		"worker_ref":    "auth/erai/api",
		"branch":        "feature/auth/erai/api",
		"worktree_path": "/wt/api",
		"base_branch":   "main",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("json[%q] = %q; want %q", k, got[k], v)
		}
	}
}

func TestWorkerAddFlags_JSONAndSpawnedBy(t *testing.T) {
	for _, name := range []string{"json", "engine", "session-id", "pane-id", "tmux-mgr-session"} {
		if featureWorkerAddCmd.Flags().Lookup(name) == nil {
			t.Errorf("worker add missing --%s", name)
		}
	}
}
