package agent

import (
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
		Status:       "RUNNING",
		StartTime:    time.Now().Truncate(time.Second), // Truncate for JSON comparison precision
		WorktreePath: "/tmp/worktree",
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
