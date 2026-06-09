package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	mcpfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// isolateMCPEnv points CLAUDE_CONFIG_DIR at an empty temp dir so
// mcp.ConfiguredCount finds no global ~/.claude.json. Returns the temp dir.
func isolateMCPEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir) // empty dir → no .claude.json
	t.Setenv("XDG_CACHE_HOME", dir)    // isolate the mcp ActiveCount cache
	return dir
}

func TestAI_NoPayload_Omits(t *testing.T) {
	isolateMCPEnv(t)
	st := asciiStyle()
	seg := NewAISegment(payload.Payload{}, "", nil, mcp.ActiveCountOptions{})
	if _, _, ok := seg.Render(context.Background(), st, 0); ok {
		t.Error("ai: empty payload should self-omit (ok=false)")
	}
}

func TestAI_FullPayload_ASCII(t *testing.T) {
	cwd := isolateMCPEnv(t)
	// Provide a .mcp.json with 2 servers so ConfiguredCount == 2.
	writeMcpJSON(t, cwd, 2)

	st := asciiStyle()
	// Active count via fake: one connected (✓) line, cache disabled by
	// pointing at a fresh file that doesn't exist yet.
	r := &mcpfake.Runner{Default: mcpfake.Response{Stdout: []byte("srvA: x - ✓ Connected\nsrvB: y - ✗ Failed\n")}}
	opts := mcp.ActiveCountOptions{CacheFile: filepath.Join(cwd, "active.json")}

	seg := NewAISegment(samplePayload(), cwd, r, opts)
	got, colorKey, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("ai: full payload should render (ok=true)")
	}
	if colorKey != "ai" {
		t.Errorf("ai: want colorKey=ai, got %q", colorKey)
	}
	for _, want := range []string{"Opus 4.7", "42%", "84k", "200k", "1/2", "5h 80%", "7d 15%"} {
		if !strings.Contains(got, want) {
			t.Errorf("ai output %q missing %q", got, want)
		}
	}
}

func TestAI_NilFieldsSkippedGracefully(t *testing.T) {
	isolateMCPEnv(t)
	st := asciiStyle()
	// Only a model name; context + rate limits absent.
	p := payload.Payload{Model: &payload.Model{DisplayName: strptr("Sonnet")}}
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	got, _, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("ai: model-only payload should render")
	}
	if !strings.Contains(got, "Sonnet") {
		t.Errorf("ai: want model name, got %q", got)
	}
	if strings.Contains(got, "%") {
		t.Errorf("ai: should not show percentages when ctx/limits nil, got %q", got)
	}
}

func TestAI_ModelOnlyNoName_ShowsAnchorWhenOtherDataPresent(t *testing.T) {
	isolateMCPEnv(t)
	st := asciiStyle()
	p := payload.Payload{
		ContextWindow: &payload.ContextWindow{UsedPercentage: f64ptr(10)},
	}
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	got, _, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("ai: ctx-only payload should render")
	}
	if !strings.Contains(got, "10%") {
		t.Errorf("ai: want 10%%, got %q", got)
	}
}

// writeMcpJSON writes a .mcp.json with n server keys into dir.
func writeMcpJSON(t *testing.T, dir string, n int) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(`{"mcpServers":{`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"s`)
		sb.WriteByte(byte('0' + i))
		sb.WriteString(`":{}`)
	}
	sb.WriteString("}}")
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}
}
