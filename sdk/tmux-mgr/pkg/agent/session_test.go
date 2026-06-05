package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	sessionID := "test-session-123456789"

	s := Session{
		SessionID:    sessionID,
		AgentName:    "test-agent",
		Status:       StatusRunning,
		StartTime:    time.Now().Truncate(time.Second), // Truncate for JSON comparison precision
		WorktreePath: "/tmp/worktree",
		PaneID:       "%99",
	}

	// Test SaveSession
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Test LoadSession
	loadedSession, err := LoadSession(sessionID)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loadedSession.SessionID != s.SessionID {
		t.Errorf("Expected SessionID %s, got %s", s.SessionID, loadedSession.SessionID)
	}
	if loadedSession.PaneID != s.PaneID {
		t.Errorf("Expected PaneID %s, got %s", s.PaneID, loadedSession.PaneID)
	}

	// Test ListSessions
	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	found := false
	for _, ls := range sessions {
		if ls.SessionID == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find session %s in ListSessions", sessionID)
	}

	// Test DeleteSession
	if err := DeleteSession(sessionID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify it's deleted
	_, err = LoadSession(sessionID)
	if err == nil {
		t.Error("Expected error loading deleted session, got nil")
	}
}

func TestReconcileStatus(t *testing.T) {
	withResult := t.TempDir()
	if err := os.WriteFile(filepath.Join(withResult, "RESULT.md"), []byte("done\n"), 0644); err != nil {
		t.Fatalf("seed RESULT.md: %v", err)
	}
	withEmptyResult := t.TempDir()
	if err := os.WriteFile(filepath.Join(withEmptyResult, "RESULT.md"), []byte(""), 0644); err != nil {
		t.Fatalf("seed empty RESULT.md: %v", err)
	}
	noResult := t.TempDir()

	alive := func(id string) bool { return id == "%alive" }
	dead := func(id string) bool { return false }

	cases := []struct {
		name string
		s    Session
		fn   PaneChecker
		want string
	}{
		{
			name: "pane alive stays running",
			s:    Session{Status: StatusRunning, PaneID: "%alive", WorktreePath: withResult},
			fn:   alive,
			want: StatusRunning,
		},
		{
			name: "pane dead with RESULT.md becomes completed",
			s:    Session{Status: StatusRunning, PaneID: "%dead", WorktreePath: withResult},
			fn:   dead,
			want: StatusCompleted,
		},
		{
			name: "pane dead with empty RESULT.md becomes failed",
			s:    Session{Status: StatusRunning, PaneID: "%dead", WorktreePath: withEmptyResult},
			fn:   dead,
			want: StatusFailed,
		},
		{
			name: "pane dead without RESULT.md becomes failed",
			s:    Session{Status: StatusRunning, PaneID: "%dead", WorktreePath: noResult},
			fn:   dead,
			want: StatusFailed,
		},
		{
			name: "terminal completed is sticky",
			s:    Session{Status: StatusCompleted, PaneID: "%dead", WorktreePath: noResult},
			fn:   dead,
			want: StatusCompleted,
		},
		{
			name: "terminal failed is sticky",
			s:    Session{Status: StatusFailed, PaneID: "%alive", WorktreePath: withResult},
			fn:   alive,
			want: StatusFailed,
		},
		{
			name: "legacy session without PaneID stays running when no RESULT",
			s:    Session{Status: StatusRunning, WorktreePath: noResult},
			fn:   dead,
			want: StatusRunning,
		},
		{
			name: "legacy session without PaneID completes when RESULT exists",
			s:    Session{Status: StatusRunning, WorktreePath: withResult},
			fn:   dead,
			want: StatusCompleted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReconcileStatus(tc.s, tc.fn); got != tc.want {
				t.Errorf("ReconcileStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListSessionsFiltered_FeatureScope(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	mkResult := func(t *testing.T) string {
		t.Helper()
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "RESULT.md"), []byte("ok"), 0644); err != nil {
			t.Fatalf("seed RESULT.md: %v", err)
		}
		return d
	}

	now := time.Now().Truncate(time.Second)

	seed := func(id, workerRef string) {
		s := Session{
			SessionID:    id,
			AgentName:    "agent",
			Status:       StatusCompleted,
			StartTime:    now,
			WorktreePath: mkResult(t),
			PaneID:       "%done",
			WorkerRef:    workerRef,
		}
		if err := SaveSession(s); err != nil {
			t.Fatalf("SaveSession %s: %v", id, err)
		}
	}

	// Repo identity is re-derived from the WorkerRef feature segment.
	seed("a-session", "auth/erai/api")
	seed("b-session", "billing/erai/api")
	seed("legacy-session", "") // legacy / global (no WorkerRef)

	dead := func(id string) bool { return false }

	cases := []struct {
		name    string
		filter  string
		wantIDs []string
	}{
		{"empty filter shows all", "", []string{"a-session", "b-session", "legacy-session"}},
		{"feature auth returns auth + legacy", "auth", []string{"a-session", "legacy-session"}},
		{"feature billing returns billing + legacy", "billing", []string{"b-session", "legacy-session"}},
		{"unknown feature returns only legacy", "nope", []string{"legacy-session"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ListSessionsFiltered(dead, tc.filter)
			if err != nil {
				t.Fatalf("ListSessionsFiltered: %v", err)
			}
			gotIDs := make([]string, 0, len(got))
			for _, s := range got {
				gotIDs = append(gotIDs, s.SessionID)
			}
			if !equalStringSets(gotIDs, tc.wantIDs) {
				t.Errorf("got %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

func TestFeatureOf(t *testing.T) {
	cases := map[string]string{
		"auth/erai/api":      "auth",
		"auth/erai/api-moss": "auth",
		"solo":               "solo",
		"":                   "",
	}
	for ref, want := range cases {
		if got := FeatureOf(ref); got != want {
			t.Errorf("FeatureOf(%q) = %q; want %q", ref, got, want)
		}
	}
}

func TestSession_WorkerRefRoundTripAndNoRepoRoot(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	s := Session{
		SessionID:    "s1",
		AgentName:    "agent",
		Status:       StatusRunning,
		StartTime:    time.Now().Truncate(time.Second),
		WorktreePath: "/wt/api",
		PaneID:       "%1",
		WorkerRef:    "auth/erai/api",
	}
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Round-trip preserves WorkerRef.
	got, err := LoadSession("s1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.WorkerRef != "auth/erai/api" {
		t.Errorf("WorkerRef = %q; want auth/erai/api", got.WorkerRef)
	}

	// The serialized form must not carry the dropped repoRoot key.
	dir, _ := GetSessionsDir()
	data, err := os.ReadFile(filepath.Join(dir, "s1.json"))
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if strings.Contains(string(data), "repoRoot") {
		t.Errorf("session JSON should not contain repoRoot:\n%s", data)
	}
	if !strings.Contains(string(data), "workerRef") {
		t.Errorf("session JSON should contain workerRef:\n%s", data)
	}
}

func TestLoadSession_BackCompatLegacyRepoRoot(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	dir, err := GetSessionsDir()
	if err != nil {
		t.Fatalf("GetSessionsDir: %v", err)
	}
	// A pre-refactor session file: has repoRoot, no workerRef. Loading must not
	// error; the unknown repoRoot key is ignored and WorkerRef stays empty.
	legacy := `{"sessionId":"old","agentName":"a","status":"RUNNING","startTime":"2026-05-21T00:00:00Z","worktreePath":"/wt/old","paneId":"%9","repoRoot":"/repo/old"}`
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte(legacy), 0644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	got, err := LoadSession("old")
	if err != nil {
		t.Fatalf("LoadSession(legacy): %v", err)
	}
	if got.WorkerRef != "" {
		t.Errorf("legacy session WorkerRef = %q; want empty", got.WorkerRef)
	}
	if got.SessionID != "old" || got.WorktreePath != "/wt/old" {
		t.Errorf("legacy fields not preserved: %+v", got)
	}
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
		if m[s] < 0 {
			return false
		}
	}
	return true
}

func TestListSessionsReconciled_PersistsTerminalStates(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "RESULT.md"), []byte("ok"), 0644); err != nil {
		t.Fatalf("seed RESULT.md: %v", err)
	}

	s := Session{
		SessionID:    "reconcile-test",
		AgentName:    "test",
		Status:       StatusRunning,
		StartTime:    time.Now().Truncate(time.Second),
		WorktreePath: worktree,
		PaneID:       "%gone",
	}
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	dead := func(id string) bool { return false }
	sessions, err := ListSessionsReconciled(dead)
	if err != nil {
		t.Fatalf("ListSessionsReconciled: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Status != StatusCompleted {
		t.Fatalf("expected one COMPLETED session, got %+v", sessions)
	}

	// Reload from disk: terminal state should be persisted.
	loaded, err := LoadSession("reconcile-test")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.Status != StatusCompleted {
		t.Errorf("expected persisted COMPLETED, got %q", loaded.Status)
	}
}
