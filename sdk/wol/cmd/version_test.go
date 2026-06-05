package cmd

import (
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := VersionInfo{
		Version:     "0.1.0",
		Description: "Wake-on-LAN",
	}

	if info.Description != "Wake-on-LAN" {
		t.Errorf("Expected description 'Wake-on-LAN', got %s", info.Description)
	}
}
