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
	"unicode"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
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
	// prio is the drop priority (config.Segment.EffectivePriority).
	prio int
}

// priority implements prioritized.
func (d *aiData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityAI
}

// detect implements detectable for AISegment. Runs all MCP subprocess I/O once.
func (s *AISegment) detect(ctx context.Context) (segmentData, bool) {
	p := s.Payload
	if p.Model == nil && p.ContextWindow == nil && p.RateLimits == nil {
		return nil, false
	}

	d := &aiData{hasPayload: true, mcpActive: -1, prio: s.Priority}

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
		// The CANONICAL name at every level (spec UC-4: "Claude Opus 4.8 (1M
		// context)" RENDERS AS "Opus 4.8" — the vendor prefix and the context
		// parenthetical are 20 columns of noise the user did not ask for), and the
		// budget-capped short form once compaction bites.
		name := canonicalModelName(d.modelName)
		if level >= 3 {
			name = shortenModelName(d.modelName)
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

// modelBudget is the display-width cap for a shortened model name.
const modelBudget = 8

// modelFamilies is the alias table: the model families we know how to name.
// Matching is case-insensitive. The value is the canonical casing to render.
//
// Order matters only for readability; the tokens are mutually exclusive.
var modelFamilies = map[string]string{
	"opus":   "Opus",
	"sonnet": "Sonnet",
	"haiku":  "Haiku",
	"gemini": "Gemini",
	"gpt":    "GPT",
}

// shortenModelName returns a compact label for a model display name.
//
// # The bug this replaces
//
// The shipped implementation returned the LAST whitespace-delimited word. For
// the actual display name Claude Code sends —
//
//	"Claude Opus 4.8 (1M context)"
//
// — that word is "context)". The live status line read `🤖 context) 🧠 42%`: a
// stray parenthesis fragment where the model name should be. The rule was never
// right; it merely happened to look right for the two-word names it was written
// against ("Opus 4.7" → "4.7" is already wrong, just less visibly).
//
// It also cut with `name[:8]` — a BYTE slice, which splits a multi-byte rune and
// emits U+FFFD.
//
// # The rule now
//
//  1. Already within budget → return unchanged.
//  2. ALIAS TABLE: find the family token (Opus/Sonnet/Haiku/Gemini/GPT), and keep
//     it together with the version token that follows it → "Opus 4.8".
//  3. Otherwise fall back to the FIRST meaningful token — the head of the name,
//     which is where a name's identity lives, not the tail.
//  4. Cut to budget grapheme-safely, never mid-rune.
func shortenModelName(name string) string {
	if name == "" {
		return ""
	}

	// 2. The alias table. When it hits, its answer is already the shortest HONEST
	//    name — no budget cut is applied on top, because "Sonnet 4.5" truncated to
	//    8 columns ("Sonnet …") is worse than the two extra columns it costs.
	if c := canonicalModelName(name); c != name {
		return c
	}

	// 1. Already within budget → unchanged.
	if term.DisplayWidth(name) <= modelBudget {
		return name
	}

	tokens := strings.Fields(name)

	// 3. The first MEANINGFUL token.
	for _, tok := range tokens {
		if isMeaningfulToken(tok) {
			return truncateText(trimTokenPunct(tok), modelBudget)
		}
	}

	// 4. No meaningful token (e.g. a single unbroken string): rune-safe cut.
	return truncateText(name, modelBudget)
}

// isMeaningfulToken reports whether tok can stand in as the model's name.
//
// A PARENTHESIZED token is decoration, not identity — "(beta)", "(1M", "(fast)".
// So is a token with no letters in it (a bare version number is a qualifier of a
// name, not a name). Treating decoration as identity is precisely the mistake
// that rendered "context)" as a model name; the fallback must not repeat it in a
// different costume.
func isMeaningfulToken(tok string) bool {
	if tok == "" {
		return false
	}
	switch []rune(tok)[0] {
	case '(', '[', '{':
		return false
	}
	for _, r := range trimTokenPunct(tok) {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// canonicalModelName is the model's name with the vendor prefix and the trailing
// marketing/context parenthetical removed:
//
//	"Claude Opus 4.8 (1M context)" → "Opus 4.8"
//	"Gemini 2.5 Pro"               → "Gemini 2.5"
//
// It looks up the family token in the alias table and keeps it together with the
// version token that follows.
//
// A name whose family we do NOT recognise is returned UNCHANGED. We normalise
// what we understand and refuse to guess at what we do not — mangling an unknown
// vendor's name is how the status line ends up lying about which model is
// answering, which is the one thing this segment exists to tell the truth about.
func canonicalModelName(name string) string {
	tokens := strings.Fields(name)
	for i, tok := range tokens {
		family, ok := modelFamilies[strings.ToLower(trimTokenPunct(tok))]
		if !ok {
			continue
		}
		if i+1 < len(tokens) {
			if v := trimTokenPunct(tokens[i+1]); isVersionToken(v) {
				return family + " " + v
			}
		}
		return family
	}
	return name
}

// trimTokenPunct strips surrounding punctuation from a token — "(1M" → "1M",
// "context)" → "context" — so the alias table and the version test see the word
// rather than the typography around it.
func trimTokenPunct(tok string) string {
	return strings.TrimFunc(tok, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// isVersionToken reports whether tok looks like a model version — it starts with
// a digit ("4.8", "2.5", "5"). Anything else after the family name (a marketing
// word like "Pro" or "Turbo", or a parenthetical) is not the version.
func isVersionToken(tok string) bool {
	if tok == "" {
		return false
	}
	return unicode.IsDigit([]rune(tok)[0])
}
