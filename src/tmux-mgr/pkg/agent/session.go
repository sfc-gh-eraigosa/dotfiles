package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionsDir = ".config/tmux-mgr/sessions"

// Session represents an active or completed agent session.
type Session struct {
	SessionID    string    `json:"sessionId"`
	AgentName    string    `json:"agentName"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"startTime"`
	WorktreePath string    `json:"worktreePath"`
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

// DeleteSession removes a session file from disk.
func DeleteSession(sessionID string) error {
	dir, err := GetSessionsDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, sessionID+".json")
	return os.Remove(path)
}
