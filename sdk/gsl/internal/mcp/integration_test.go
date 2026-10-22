//go:build integration

package mcp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// E9 — the drift alarm. Runs the REAL `claude mcp list` (go test -tags
// integration ./internal/mcp/) and fails when its output stops parsing: every
// line that looks like a server must classify to a known state. Not in the
// default suite — it needs claude on PATH and dials every server.
func TestParseStatus_RealCLI(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := NewSystemRunner().Run(ctx, "claude", "mcp", "list")
	s := ParseStatus(out)
	if err != nil && len(s.Servers) == 0 {
		t.Fatalf("claude mcp list: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "No MCP servers configured") {
		t.Skip("no MCP servers configured on this host")
	}
	if len(s.Servers) == 0 {
		t.Fatalf("no server parsed from real output — format drift?\n%s", out)
	}
	for _, sv := range s.Servers {
		if sv.State == StateUnknown {
			t.Errorf("server %q: status not recognised — format drift? (update classify and the fixture)", sv.Name)
		}
	}
	t.Logf("%d servers: %d connected, %d failed, %d needs auth (%s)", len(s.Servers), s.Connected, s.Failed, s.NeedsAuth, time.Now().Format(time.RFC3339))
}
