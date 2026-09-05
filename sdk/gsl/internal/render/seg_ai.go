package render

import (
	"context"
	"strconv"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// AISegment renders Claude-payload-derived data: the model display name, the
// context-window usage, the MCP active/configured count, and the 5h / 7d
// rate-limit usage.
//
// It is the ONLY payload-dependent segment. In Antigravity/CLI mode the payload is
// the empty struct (all pointer fields nil), so the segment self-omits
// (ok == false). Individual nil fields inside a present payload are skipped
// gracefully.
type AISegment struct {
	// Payload is the parsed Claude stdin payload (may be the empty struct).
	Payload payload.Payload
	// Cwd is the working directory used for mcp.ConfiguredCount. When empty,
	// MCP detection is skipped.
	Cwd string
	// MCP is the injected mcp.Runner used by mcp.ActiveCount. When nil, the
	// active count is omitted (configured count may still render).
	MCP mcp.Runner
	// MCPOpts is forwarded to mcp.ActiveCount (cache file / clock injection).
	MCPOpts mcp.ActiveCountOptions
	// Priority is the DROP priority used by the fit loop (config.Segment.Priority,
	// or the built-in default for this type when unset). It is independent of the
	// segment's position in the line.
	Priority int

	// Links is the link policy (Deps.Links): AI gates the model / context /
	// rate fields → Links.UsageURL. Zero value or empty UsageURL ⇒ no links.
	Links Links
}

// NewAISegment builds an AISegment.
func NewAISegment(p payload.Payload, cwd string, mcpRunner mcp.Runner, mcpOpts mcp.ActiveCountOptions) *AISegment {
	return &AISegment{Payload: p, Cwd: cwd, MCP: mcpRunner, MCPOpts: mcpOpts}
}

// Render implements Segment. It delegates to RenderLinked and discards the
// spans, so the legacy path can never drift from the detect/format path.
func (s *AISegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	text, colorKey, _, ok = s.RenderLinked(ctx, st, level)
	return text, colorKey, ok
}

// RenderLinked implements LinkedSegment: detect once, then format with spans.
func (s *AISegment) RenderLinked(ctx context.Context, st style.Style, level int) (text, colorKey string, spans []LinkSpan, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", nil, false
	}
	text, colorKey, spans = formatLinkedOf(d, st, level)
	return text, colorKey, spans, text != ""
}

// pct formats a 0–100 percentage as an integer with a trailing "%". Values are
// rounded to the nearest integer.
func pct(v float64) string {
	return strconv.Itoa(int(v+0.5)) + "%"
}

// tokenAbbrev formats a token count compactly: <1000 verbatim, ≥1000 as "Nk"
// (rounded to whole thousands), ≥1_000_000 as "Nm".
func tokenAbbrev(v float64) string {
	n := int(v + 0.5)
	switch {
	case n >= 1_000_000:
		return strconv.Itoa(n/1_000_000) + "m"
	case n >= 1000:
		return strconv.Itoa(n/1000) + "k"
	default:
		return strconv.Itoa(n)
	}
}
