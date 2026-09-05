package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
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

func TestAI_Spans_UsageFourFields(t *testing.T) {
	seg := NewAISegment(samplePayload(), "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true, UsageURL: DefaultClaudeUsageURL}
	text, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 4 {
		t.Fatalf("want model/ctx/5h/7d spans, got %+v in %q", spans, text)
	}
	// The model name opens its model page (family from the display name here);
	// context and rate fields open the usage page.
	if spans[0].URL != "https://www.anthropic.com/claude/opus" || text[spans[0].Start:spans[0].End] != "Opus 4.7" {
		t.Errorf("model span = %+v (%q)", spans[0], text[spans[0].Start:spans[0].End])
	}
	for _, sp := range spans[1:] {
		if sp.URL != DefaultClaudeUsageURL || strings.Contains(text[sp.Start:sp.End], "MCP") {
			t.Errorf("bad span %+v", sp)
		}
	}
}

func TestAI_Spans_ModelURL_Gemini(t *testing.T) {
	p := samplePayload()
	p.Model = &payload.Model{ID: strptr("Gemini 3.5 Flash (Medium)"), DisplayName: strptr("Gemini 3.5 Flash (Medium)")}
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true, UsageURL: DefaultClaudeUsageURL}
	_, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 4 || spans[0].URL != DefaultGeminiModelURL {
		t.Errorf("gemini model span: %+v", spans)
	}
}

func TestAI_Spans_ModelURL_UnknownFamilyHasNoModelLink(t *testing.T) {
	p := samplePayload()
	p.Model = &payload.Model{DisplayName: strptr("Mystery")}
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true, UsageURL: DefaultClaudeUsageURL}
	text, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 3 || strings.Contains(text[spans[0].Start:spans[0].End], "Mystery") {
		t.Errorf("unknown family: want ctx/5h/7d spans only, got %+v in %q", spans, text)
	}
}

func TestAI_Spans_ModelURL_TemplateOverride(t *testing.T) {
	p := samplePayload()
	p.Model = &payload.Model{ID: strptr("claude-fable-5-1"), DisplayName: strptr("Fable")}
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true, ModelURL: "https://x/{family}/{model_id}"}
	_, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 1 || spans[0].URL != "https://x/fable/claude-fable-5-1" {
		t.Errorf("template override (no usage URL ⇒ only the model span): %+v", spans)
	}
}

func TestBuildSegments_AIModelURLOption(t *testing.T) {
	cfg := config.Default()
	for i := range cfg.Segments {
		if cfg.Segments[i].Type == "ai" {
			cfg.Segments[i].Options = map[string]any{"model_url": "https://x/{family}"}
		}
	}
	segs := BuildSegments(cfg, Deps{Payload: samplePayload(), Links: Links{AI: true}})
	var ai *AISegment
	for _, s := range segs {
		if a, ok := s.(*AISegment); ok {
			ai = a
		}
	}
	if ai == nil || ai.Links.ModelURL != "https://x/{family}" {
		t.Fatalf("model_url option not threaded: %+v", ai)
	}
}

func TestAI_Spans_NoUsageURL(t *testing.T) {
	seg := NewAISegment(samplePayload(), "", nil, mcp.ActiveCountOptions{})
	seg.Links = Links{AI: true}
	// No usage URL ⇒ no context/rate spans; the model name still opens its
	// model page (built-in family map).
	if _, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0); len(spans) != 1 || spans[0].URL != "https://www.anthropic.com/claude/opus" {
		t.Errorf("no usage URL ⇒ only the model-page span: %+v", spans)
	}
}
