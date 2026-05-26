package render

import (
	"context"
	"strconv"
	"strings"

	"github.com/wenlock/dotfiles/gsl/internal/mcp"
	"github.com/wenlock/dotfiles/gsl/internal/payload"
	"github.com/wenlock/dotfiles/gsl/internal/style"
)

// AISegment renders Claude-payload-derived data: the model display name, the
// context-window usage, the MCP active/configured count, and the 5h / 7d
// rate-limit usage.
//
// It is the ONLY payload-dependent segment. In Gemini/CLI mode the payload is
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
}

// NewAISegment builds an AISegment.
func NewAISegment(p payload.Payload, cwd string, mcpRunner mcp.Runner, mcpOpts mcp.ActiveCountOptions) *AISegment {
	return &AISegment{Payload: p, Cwd: cwd, MCP: mcpRunner, MCPOpts: mcpOpts}
}

// Render implements Segment.
func (s *AISegment) Render(ctx context.Context, st style.Style) (string, bool) {
	p := s.Payload
	// No payload at all (Gemini/CLI mode): every payload pointer is nil.
	if p.Model == nil && p.ContextWindow == nil && p.RateLimits == nil {
		return "", false
	}

	parts := make([]string, 0, 5)

	// ── Model display name ───────────────────────────────────────────────────
	if p.Model != nil && p.Model.DisplayName != nil && *p.Model.DisplayName != "" {
		var b strings.Builder
		if g := glyph(st, "ai"); g != "" {
			b.WriteString(g)
			b.WriteString(" ")
		}
		b.WriteString(*p.Model.DisplayName)
		parts = append(parts, b.String())
	} else if g := glyph(st, "ai"); g != "" {
		// Payload present but no model name: still show the AI glyph as an
		// anchor so the segment is not empty when only ctx/limits exist.
		parts = append(parts, g)
	}

	// ── Context window ───────────────────────────────────────────────────────
	if cw := p.ContextWindow; cw != nil {
		if part := s.contextPart(st, cw); part != "" {
			parts = append(parts, part)
		}
	}

	// ── MCP active/configured ────────────────────────────────────────────────
	if part := s.mcpPart(ctx, st); part != "" {
		parts = append(parts, part)
	}

	// ── Rate limits ──────────────────────────────────────────────────────────
	if rl := p.RateLimits; rl != nil {
		if part := ratePart(st, "5h", rl.FiveHour); part != "" {
			parts = append(parts, part)
		}
		if part := ratePart(st, "7d", rl.SevenDay); part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return "", false
	}
	return paint(st, "ai", strings.Join(parts, " ")), true
}

// contextPart renders the context-window usage, e.g. "<ctx-glyph> 42% 50k/200k".
// Percentage is shown when present; the token ratio is appended when both
// totals are present.
func (s *AISegment) contextPart(st style.Style, cw *payload.ContextWindow) string {
	var b strings.Builder
	if g := glyph(st, "context"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	wrote := false
	if cw.UsedPercentage != nil {
		b.WriteString(pct(*cw.UsedPercentage))
		wrote = true
	}
	if cw.TotalInputTokens != nil && cw.ContextWindowSize != nil {
		if wrote {
			b.WriteString(" ")
		}
		b.WriteString(tokenAbbrev(*cw.TotalInputTokens))
		b.WriteString("/")
		b.WriteString(tokenAbbrev(*cw.ContextWindowSize))
		wrote = true
	}
	if !wrote {
		return ""
	}
	return b.String()
}

// mcpPart renders the MCP active/configured count, e.g. "<mcp-glyph> 3/5".
// Returns "" when neither count is available. ConfiguredCount is read from
// config files (no subprocess); ActiveCount uses the injected runner with its
// own short timeout + cache.
func (s *AISegment) mcpPart(ctx context.Context, st style.Style) string {
	if s.Cwd == "" {
		return ""
	}
	configured, cErr := mcp.ConfiguredCount(s.Cwd)
	if cErr != nil {
		return ""
	}

	active := -1
	if s.MCP != nil {
		if n, aErr := mcp.ActiveCount(ctx, s.MCP, s.MCPOpts); aErr == nil {
			active = n
		}
	}

	if configured == 0 && active <= 0 {
		return ""
	}

	var b strings.Builder
	if g := glyph(st, "mcp"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	if active >= 0 {
		b.WriteString(strconv.Itoa(active))
		b.WriteString("/")
		b.WriteString(strconv.Itoa(configured))
	} else {
		b.WriteString(strconv.Itoa(configured))
	}
	return b.String()
}

// ratePart renders a single rate-limit window, e.g. "5h 80%". Returns "" when
// the window or its percentage is nil.
func ratePart(st style.Style, label string, w *payload.RateWindow) string {
	if w == nil || w.UsedPercentage == nil {
		return ""
	}
	return label + " " + pct(*w.UsedPercentage)
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
