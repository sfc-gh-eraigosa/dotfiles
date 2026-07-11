package cmd

import (
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := VersionInfo{
		Version:     "0.1.0",
		Commit:      "123456",
		Dirty:       "true",
		BuildDate:   "2026-05-16",
		Description: "Tmux Manager",
		Path:        "/usr/local/bin/tmux-mgr",
	}

	if info.Version != "0.1.0" {
		t.Errorf("Expected version 0.1.0, got %s", info.Version)
	}
}
