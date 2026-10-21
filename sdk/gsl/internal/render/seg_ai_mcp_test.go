package render

import (
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// E13: the MCP part shrinks L0 → L4 and never renders N/0.
func TestMCPPart_Levels(t *testing.T) {
	probed := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	full := mcp.Status{Configured: 12, Connected: 6, Failed: 5, NeedsAuth: 1, FetchedAt: probed}
	for level, want := range map[int]string{0: "6✓ 5✗ 1!", 1: "6✓ 5✗", 2: "6/12", 3: "6", 4: ""} {
		if got := mcpPart(full, level); got != want {
			t.Errorf("level %d = %q, want %q", level, got, want)
		}
	}
	// Zero parts are left out of the breakdown.
	if got := mcpPart(mcp.Status{Configured: 3, Connected: 3, FetchedAt: probed}, 0); got != "3✓" {
		t.Errorf("all connected L0 = %q", got)
	}
	// The denominator is never below what the probe saw (claude.ai connectors
	// are not in any config file): 13 probed against 2 configured is 13/44,
	// never 13/2; not-configured servers do not count.
	big := mcp.Status{Configured: 2, Connected: 13, Failed: 5, NeedsAuth: 26, FetchedAt: probed,
		Servers: make([]mcp.ServerStatus, 0)}
	if got := mcpPart(big, 2); got != "13/44" {
		t.Errorf("denominator = %q, want 13/44", got)
	}
	// Never N/0, and nothing at all → omitted.
	if got := mcpPart(mcp.Status{}, 0); got != "" {
		t.Errorf("empty status = %q, want omitted", got)
	}
	// Never probed (cold cache): the count is unknown, not 0.
	cold := mcp.Status{Configured: 4, Stale: true}
	for level, want := range map[int]string{0: "?/4", 2: "?/4", 3: ""} {
		if got := mcpPart(cold, level); got != want {
			t.Errorf("cold L%d = %q, want %q", level, got, want)
		}
	}
}

// The AI segment renders the part after the context usage, with its glyph.
func TestAISegment_RendersMCPPart(t *testing.T) {
	d := &aiData{hasPayload: true, modelName: "Claude Opus 4.8", mcp: mcp.Status{Configured: 12, Connected: 6, Failed: 5, NeedsAuth: 1, FetchedAt: time.Now()}}
	st := style.Style{Icons: map[string]string{"mcp": "🔌"}}
	text, _, _ := d.formatLinked(st, 0)
	if want := "🔌 6✓ 5✗ 1!"; !contains(text, want) {
		t.Errorf("text = %q, want it to hold %q", text, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// mcpOneOfTwo is a fixed MCP status for segment tests: 2 configured, one
// connected and one failed.
func mcpOneOfTwo() mcp.Options {
	return mcp.Options{Fixed: &mcp.Status{Configured: 2, Connected: 1, Failed: 1, FetchedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}}
}
