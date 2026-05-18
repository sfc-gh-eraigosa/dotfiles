package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const sessionsDir = ".config/tmux-mgr/sessions"

// Session lifecycle states. RUNNING is the only non-terminal state; COMPLETED
// and FAILED are settled and will not be re-reconciled.
const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

// Session represents an active or completed agent session.
type Session struct {
	SessionID    string    `json:"sessionId"`
	AgentName    string    `json:"agentName"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"startTime"`
	WorktreePath string    `json:"worktreePath"`
	PaneID       string    `json:"paneId,omitempty"`
	// RepoRoot is the absolute path of the git repository the session was
	// started from. Empty for legacy sessions (always visible regardless of
	// filter) and for any future $HOME-only sessions.
	RepoRoot string `json:"repoRoot,omitempty"`
}

// PaneChecker reports whether a tmux pane is still alive. Injected for tests.
type PaneChecker func(paneID string) bool

// DefaultPaneChecker queries tmux for the set of live pane IDs.
func DefaultPaneChecker(paneID string) bool {
	if paneID == "" {
		return false
	}
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		return false
	}
	return slices.Contains(strings.Fields(string(out)), paneID)
}

// ReconcileStatus derives the live status of a session by checking pane liveness
// and RESULT.md presence. Terminal states (COMPLETED/FAILED) are returned as-is.
// When the pane is gone, RESULT.md presence decides COMPLETED vs FAILED.
func ReconcileStatus(s Session, isAlive PaneChecker) string {
	if s.Status == StatusCompleted || s.Status == StatusFailed {
		return s.Status
	}
	if s.PaneID != "" && isAlive(s.PaneID) {
		return StatusRunning
	}
	if s.WorktreePath != "" {
		if info, err := os.Stat(filepath.Join(s.WorktreePath, "RESULT.md")); err == nil && info.Size() > 0 {
			return StatusCompleted
		}
	}
	if s.PaneID == "" {
		// No pane recorded (older session) and no RESULT.md — keep RUNNING so we
		// don't mark legacy sessions as failed retroactively.
		return s.Status
	}
	return StatusFailed
}

// GetSessionsDir returns the absolute path to the sessions directory.
func GetSessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, sessionsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return dir, nil
}

// SaveSession saves a session to disk.
func SaveSession(s Session) error {
	dir, err := GetSessionsDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	path := filepath.Join(dir, s.SessionID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// LoadSession loads a session from disk by its ID.
func LoadSession(sessionID string) (*Session, error) {
	dir, err := GetSessionsDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file %s: %w", path, err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session %s: %w", sessionID, err)
	}

	return &s, nil
}

// ListSessions returns all stored sessions.
func ListSessions() ([]Session, error) {
	dir, err := GetSessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []Session
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			sessionID := entry.Name()[:len(entry.Name())-5] // trim .json
			s, err := LoadSession(sessionID)
			if err == nil {
				sessions = append(sessions, *s)
			}
		}
	}

	return sessions, nil
}

// ListSessionsReconciled returns all stored sessions with their status reconciled
// against tmux + RESULT.md state. Sessions that transition to a terminal state
// are persisted back so subsequent calls are cheap.
func ListSessionsReconciled(isAlive PaneChecker) ([]Session, error) {
	return ListSessionsFiltered(isAlive, "")
}

// ListSessionsFiltered is ListSessionsReconciled with an additional repo filter.
// When repoRoot is empty, no filtering is applied. When non-empty, only sessions
// whose RepoRoot matches (or whose RepoRoot is empty — legacy/global sessions)
// are returned. The reconcile-and-persist semantics still apply to every session
// on disk so terminal transitions settle regardless of which filter is active.
func ListSessionsFiltered(isAlive PaneChecker, repoRoot string) ([]Session, error) {
	sessions, err := ListSessions()
	if err != nil {
		return nil, err
	}
	out := sessions[:0]
	for i := range sessions {
		newStatus := ReconcileStatus(sessions[i], isAlive)
		if newStatus != sessions[i].Status {
			sessions[i].Status = newStatus
			if newStatus == StatusCompleted || newStatus == StatusFailed {
				_ = SaveSession(sessions[i])
			}
		}
		if repoRoot == "" || sessions[i].RepoRoot == "" || sessions[i].RepoRoot == repoRoot {
			out = append(out, sessions[i])
		}
	}
	return out, nil
}

// DeleteSession removes a session file from disk.
func DeleteSession(sessionID string) error {
	dir, err := GetSessionsDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, sessionID+".json")
	return os.Remove(path)
}
