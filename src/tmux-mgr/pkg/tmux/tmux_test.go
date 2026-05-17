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
