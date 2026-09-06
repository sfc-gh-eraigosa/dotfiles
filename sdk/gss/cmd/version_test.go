package cmd

import (
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := VersionInfo{
		Version:     "1.0.0",
		Commit:      "abcdef",
		Dirty:       "false",
		BuildDate:   "2026-05-16",
		Description: "Test Description",
		Path:        "/tmp/test",
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}
	if info.Commit != "abcdef" {
		t.Errorf("Expected commit abcdef, got %s", info.Commit)
	}
}
