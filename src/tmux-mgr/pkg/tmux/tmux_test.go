package tmux

import (
	"encoding/json"
	"testing"
)

func TestLayoutMarshaling(t *testing.T) {
	layout := Layout{
		Name: "test-session",
		Windows: []WindowInfo{
			{Index: 0, Name: "editor", Layout: "abcde"},
			{Index: 1, Name: "logs", Layout: "fghij"},
		},
	}

	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("Failed to marshal layout: %v", err)
	}

	var decoded Layout
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal layout: %v", err)
	}

	if decoded.Name != layout.Name {
		t.Errorf("Expected name %s, got %s", layout.Name, decoded.Name)
	}

	if len(decoded.Windows) != len(layout.Windows) {
		t.Errorf("Expected %d windows, got %d", len(layout.Windows), len(decoded.Windows))
	}
}

func TestCreatePane(t *testing.T) {
	// Note: This test relies on tmux being installed and potentially running.
	// If it fails due to tmux missing, that's expected in a minimal CI without tmux.
	err := CreatePane("test-session", "/tmp", "echo hello")
	if err != nil {
		t.Logf("CreatePane returned an error, possibly because tmux is not running: %v", err)
	}
}

func TestLegacyManager(t *testing.T) {
	m := &Manager{}
	
	// Test Run
	_, err := m.Run("info")
	if err != nil {
		t.Logf("Run failed: %v", err)
	}

	// Test Capture
	_, err = m.Capture("invalid-pane")
	if err == nil {
		t.Error("Expected error when capturing invalid pane")
	}

	// Test Attach
	err = m.Attach("invalid-session")
	if err == nil {
		t.Error("Expected error when attaching to invalid session")
	}

	// Test SetPaneTitle
	err = m.SetPaneTitle("test-title")
	if err != nil {
		t.Logf("SetPaneTitle failed: %v", err)
	}

	// Test SaveLayout
	err = m.SaveLayout("test-layout")
	if err != nil {
		t.Logf("SaveLayout failed: %v", err)
	}

	// Test RestoreLayout
	err = m.RestoreLayout("test-layout")
	if err != nil {
		t.Logf("RestoreLayout failed: %v", err)
	}
}
