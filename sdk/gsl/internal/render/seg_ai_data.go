package render

// seg_ai_data.go — aiData segmentData + AISegment.detect().
//
// Compaction levels for the AI segment:
//   level 0: <glyph> <model> <ctx%> <token-ratio> [<mcp>] [5h%] [7d%]
//   level 1: drop 7d rate limit
//   level 2: drop token ratio (keep ctx% only)
//   level 3: shorten model name + drop MCP count

import (
	"context"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// aiData is the detect-once intermediate for the AI segment.
type aiData struct {
	modelName     string
	ctxPct        *float64
	tokenUsed     *float64
	tokenTotal    *float64
	mcpActive     int // -1 = unavailable
	mcpConfigured int
	rate5h        *float64
	rate7d        *float64
	hasPayload    bool
}

// detect implements detectable for AISegment. Runs all MCP subprocess I/O once.
func (s *AISegment) detect(ctx context.Context) (segmentData, bool) {
	p := s.Payload
	if p.Model == nil && p.ContextWindow == nil && p.RateLimits == nil {
		return nil, false
	}

	d := &aiData{hasPayload: true, mcpActive: -1}

	if p.Model != nil && p.Model.DisplayName != nil {
		d.modelName = *p.Model.DisplayName
	}
	if cw := p.ContextWindow; cw != nil {
		d.ctxPct = cw.UsedPercentage
		d.tokenUsed = cw.TotalInputTokens
		d.tokenTotal = cw.ContextWindowSize
	}
	if rl := p.RateLimits; rl != nil {
		if rl.FiveHour != nil {
			d.rate5h = rl.FiveHour.UsedPercentage
		}
		if rl.SevenDay != nil {
			d.rate7d = rl.SevenDay.UsedPercentage
		}
	}

	// MCP detection (subprocess I/O happens here).
	if s.Cwd != "" {
		if n, err := mcp.ConfiguredCount(s.Cwd); err == nil {
			d.mcpConfigured = n
			if s.MCP != nil {
				if active, aErr := mcp.ActiveCount(ctx, s.MCP, s.MCPOpts); aErr == nil {
					d.mcpActive = active
				}
			}
		}
	}

	return d, true
}

// format implements segmentData.format for aiData. Pure; no I/O.
func (d *aiData) format(st style.Style, level int) (text, colorKey string) {
	if !d.hasPayload {
		return "", ""
	}

	parts := make([]string, 0, 6)

	// ── Model display name ──────────────────────────────────────────────────
	if d.modelName != "" {
		var b strings.Builder
		if g := glyph(st, "ai"); g != "" {
			b.WriteString(g)
			b.WriteString(" ")
		}
		name := d.modelName
		if level >= 3 {
			name = shortenModelName(name)
		}
		b.WriteString(name)
		parts = append(parts, b.String())
	} else if g := glyph(st, "ai"); g != "" {
		parts = append(parts, g)
	}

	// ── Context window ──────────────────────────────────────────────────────
	if d.ctxPct != nil {
		var b strings.Builder
		if g := glyph(st, "context"); g != "" {
			b.WriteString(g)
			b.WriteString(" ")
		}
		b.WriteString(pct(*d.ctxPct))
		// Token ratio: levels 0–1 only.
		if level <= 1 && d.tokenUsed != nil && d.tokenTotal != nil {
			b.WriteString(" ")
			b.WriteString(tokenAbbrev(*d.tokenUsed))
			b.WriteString("/")
			b.WriteString(tokenAbbrev(*d.tokenTotal))
		}
		parts = append(parts, b.String())
	}

	// ── MCP count (dropped at level 3) ─────────────────────────────────────
	if level < 3 && (d.mcpConfigured > 0 || d.mcpActive > 0) {
		var b strings.Builder
		if g := glyph(st, "mcp"); g != "" {
			b.WriteString(g)
			b.WriteString(" ")
		}
		if d.mcpActive >= 0 {
			b.WriteString(strconv.Itoa(d.mcpActive))
			b.WriteString("/")
			b.WriteString(strconv.Itoa(d.mcpConfigured))
		} else {
			b.WriteString(strconv.Itoa(d.mcpConfigured))
		}
		parts = append(parts, b.String())
	}

	// ── Rate limits ──────────────────────────────────────────────────────────
	// Level 0: 5h + 7d (matching the original seg_ai.go order); level 1: 5h only; level 2+: none.
	if level <= 1 && d.rate5h != nil {
		parts = append(parts, "5h "+pct(*d.rate5h))
	}
	if level == 0 && d.rate7d != nil {
		parts = append(parts, "7d "+pct(*d.rate7d))
	}

	if len(parts) == 0 {
		return "", ""
	}
	return strings.Join(parts, " "), "ai"
}

// shortenModelName returns a compact label for a model display name.
//  1. If ≤ 8 chars, return as-is.
//  2. Return the last whitespace-delimited word.
//  3. Truncate to 8 chars.
func shortenModelName(name string) string {
	if len(name) <= 8 {
		return name
	}
	parts := strings.Fields(name)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != name {
			return last
		}
	}
	return name[:8]
}
