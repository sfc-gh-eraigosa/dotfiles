package mcp

import (
	"os"
	"testing"
)

// E8: the verbatim capture classifies by status word, not by sigil.
func TestParseStatus_Golden(t *testing.T) {
	out, err := os.ReadFile("testdata/mcp-list-real.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := ParseStatus(out)
	if s.Connected != 13 || s.Failed != 5 || s.NeedsAuth != 26 {
		t.Fatalf("connected/failed/auth = %d/%d/%d, want 13/5/26", s.Connected, s.Failed, s.NeedsAuth)
	}
	if len(s.Servers) != 66 {
		t.Fatalf("servers = %d, want 66", len(s.Servers))
	}
	notConfigured := 0
	for _, sv := range s.Servers {
		if sv.State == StateNotConfigured {
			notConfigured++
		}
		if sv.Name == "" {
			t.Errorf("a server has no name: %+v", sv)
		}
	}
	if notConfigured != 22 {
		t.Errorf("not configured = %d, want 22", notConfigured)
	}
	if s.Servers[0].Name != "claude.ai Connector 01" || s.Servers[0].State != StateConnected {
		t.Errorf("first server = %+v", s.Servers[0])
	}
	if s.Configured != 0 {
		t.Errorf("ParseStatus must not set Configured (config scan does), got %d", s.Configured)
	}
}

// The status word decides; sigils are secondary and any of them may change.
func TestParseStatus_KeywordsNotSigils(t *testing.T) {
	cases := map[string]State{
		"a: x - ✓ Connected":                  StateConnected,
		"a: x - ✔ Connected":                  StateConnected,
		"a: x - ✅ connected":                  StateConnected,
		"a: x - Connected":                    StateConnected,
		"a: x - ✗ Failed to connect":          StateFailed,
		"a: x - ✘ Failed to connect — HTTP 4": StateFailed,
		"a: x - ! Needs authentication":       StateNeedsAuth,
		"a: x - ⏳ Connecting…":                StateConnecting,
		"a: x - Not configured":               StateNotConfigured,
		"a: x - Not connected":                StateFailed,
		"a: x - something new":                StateUnknown,
		// the LAST " - " splits: a name or URL may hold " - " too
		"my - server: https://h/x - y - ✔ Connected": StateConnected,
	}
	for line, want := range cases {
		s := ParseStatus([]byte(line + "\n"))
		if len(s.Servers) != 1 || s.Servers[0].State != want {
			t.Errorf("%q → %+v, want %v", line, s.Servers, want)
		}
	}
	if s := ParseStatus([]byte("my - server: https://h/x - y - ✔ Connected\n")); s.Servers[0].Name != "my - server" {
		t.Errorf("name = %q", s.Servers[0].Name)
	}
}

func TestParseStatus_IgnoresNonStatusLines(t *testing.T) {
	s := ParseStatus([]byte("Checking MCP server health…\n\nNo MCP servers configured. Use `claude mcp add` to add a server.\n"))
	if len(s.Servers) != 0 || !s.Empty() {
		t.Errorf("non-status lines produced %+v", s)
	}
}
