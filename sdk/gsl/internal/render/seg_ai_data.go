package render

// seg_ai_data.go — aiData segmentData + AISegment.detect().
//
// Compaction levels for the AI segment:
//   level 0: <glyph> <model> <ctx%> <token-ratio> [<mcp>] [5h%] [7d%]
//   level 1: drop 7d rate limit
//   level 2: drop token ratio (keep ctx% only)
//   level 3: shorten model name; MCP down to its connected count
//   level 4: drop MCP (see mcpPart for its own ladder)

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
	modelName  string
	modelID    string
	ctxPct     *float64
	tokenUsed  *float64
	tokenTotal *float64
	mcp        mcp.Status
	part       string // "" = all (the legacy ai segment); else one piece (#426)
	rate5h     *float64
	rate7d     *float64
	hasPayload bool
	// prio is the drop priority (config.Segment.EffectivePriority).
	prio int
	// links is the policy the spans are gated on.
	links Links
}

// priority implements prioritized.
func (d *aiData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityAI
}

// detect implements detectable for AISegment. It reads the MCP cache (never
// probes) and, when the cache is due, asks MCPSpawn for an out-of-band refresh.
func (s *AISegment) detect(ctx context.Context) (segmentData, bool) {
	p := s.Payload
	if p.Model == nil && p.ContextWindow == nil && p.RateLimits == nil {
		return nil, false
	}

	d := &aiData{hasPayload: true, prio: s.Priority, links: s.Links, part: s.Part}

	if p.Model != nil && p.Model.DisplayName != nil {
		d.modelName = *p.Model.DisplayName
	}
	if p.Model != nil && p.Model.ID != nil {
		d.modelID = *p.Model.ID
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

	// MCP: cache only (mcp.Get never spawns); a due refresh runs out of band.
	if s.Cwd != "" && (s.Part == "" || s.Part == "mcp") {
		st, _ := mcp.Get(ctx, s.Cwd, s.MCPOpts)
		d.mcp = st
		if st.RefreshDue && s.MCPSpawn != nil {
			s.spawnOnce.Do(func() { _ = s.MCPSpawn(s.Cwd) })
		}
	}

	return d, true
}

// format implements segmentData.format for aiData. Pure; no I/O.
func (d *aiData) format(st style.Style, level int) (text, colorKey string) {
	text, colorKey, _ = d.formatLinked(st, level)
	return text, colorKey
}

// formatLinked implements linkedFormatter. Links (AI family): the model name →
// its model page (ModelURL: family from the id, else the display name); the
// context usage and each rate-limit field → the usage page. The MCP count has
// no web home and stays plain.
func (d *aiData) formatLinked(st style.Style, level int) (string, string, []LinkSpan) {
	if !d.hasPayload {
		return "", "", nil
	}

	usage, modelPage := "", ""
	if d.links.AI {
		usage = d.links.UsageURL
		modelPage = ModelURL(d.links.ModelURL, d.modelID, d.modelName)
	}
	var sb spanBuilder
	sep := func() {
		if sb.len() > 0 {
			sb.write(" ")
		}
	}
	show := func(part string) bool { return d.part == "" || d.part == part }

	// ── Model display name ──────────────────────────────────────────────────
	if show("model") {
		if d.modelName != "" {
			if g := glyph(st, "ai"); g != "" {
				sb.write(g)
				sb.write(" ")
			}
			// The CANONICAL name at every level (spec UC-4: "Claude Opus 4.8 (1M
			// context)" RENDERS AS "Opus 4.8" — the vendor prefix and the context
			// parenthetical are 20 columns of noise the user did not ask for), and
			// the budget-capped short form once compaction bites.
			name := canonicalModelName(d.modelName)
			if level >= 3 {
				name = shortenModelName(d.modelName)
			}
			sb.linked(name, modelPage)
		} else if g := glyph(st, "ai"); g != "" {
			sb.write(g)
		}
	}

	// ── Context window ──────────────────────────────────────────────────────
	if show("context") && d.ctxPct != nil {
		sep()
		if g := glyph(st, "context"); g != "" {
			sb.write(g)
			sb.write(" ")
		}
		ctx := pct(*d.ctxPct)
		// Token ratio: levels 0–1 only.
		if level <= 1 && d.tokenUsed != nil && d.tokenTotal != nil {
			ctx += " " + tokenAbbrev(*d.tokenUsed) + "/" + tokenAbbrev(*d.tokenTotal)
		}
		sb.linked(ctx, usage)
	}

	// ── MCP health (its own ladder; see mcpPart) ────────────────────────────
	if runs := mcpParts(d.mcp, level); show("mcp") && len(runs) > 0 {
		sep()
		if g := glyph(st, "mcp"); g != "" {
			sb.write(g)
			sb.write(" ")
		}
		for i, r := range runs {
			if i > 0 {
				sb.write(" ")
			}
			sb.styled(r.text, r.role, "")
		}
	}

	// ── Rate limits ──────────────────────────────────────────────────────────
	// Level 0: 5h + 7d (matching the original seg_ai.go order); level 1: 5h only; level 2+: none.
	if show("limits") && level <= 1 && d.rate5h != nil {
		sep()
		sb.linked("5h "+pct(*d.rate5h), usage)
	}
	if show("limits") && level == 0 && d.rate7d != nil {
		sep()
		sb.linked("7d "+pct(*d.rate7d), usage)
	}

	if sb.len() == 0 {
		return "", "", nil
	}
	return sb.String(), "ai", sb.spans
}

// mcpPart is the MCP health for a compaction level (plan F8):
//
//	L0 "6✓ 5✗ 1!" · L1 "6✓ 5✗" · L2 "6/12" · L3 "6" · L4 dropped
//
// Zero parts are left out of the breakdown. The denominator is the larger of
// the configured count and what the probe saw, since claude.ai connectors are
// in no config file — so it is never below the numerator and never 0. A
// status never probed (cold cache) shows "?/N" until L3, and nothing at all
// is shown for an empty status.
func mcpPart(s mcp.Status, level int) string {
	var b strings.Builder
	for i, p := range mcpParts(s, level) {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(p.text)
	}
	return b.String()
}

// mcpRun is one piece of the MCP part with the role it is painted in.
type mcpRun struct{ text, role string }

// mcpParts is mcpPart as runs: connected ok, failed err, needs-auth warn.
func mcpParts(s mcp.Status, level int) []mcpRun {
	probed := s.Connected + s.Failed + s.NeedsAuth
	total := max(s.Configured, probed)
	if total == 0 || level >= 4 {
		return nil
	}
	if s.FetchedAt.IsZero() {
		if level >= 3 {
			return nil
		}
		return []mcpRun{{"?/" + strconv.Itoa(total), string(style.RoleMuted)}}
	}
	switch level {
	case 0, 1:
		var runs []mcpRun
		add := func(n int, mark string, role style.Role) {
			if n > 0 {
				runs = append(runs, mcpRun{strconv.Itoa(n) + mark, string(role)})
			}
		}
		add(s.Connected, "✓", style.RoleOK)
		add(s.Failed, "✗", style.RoleErr)
		if level == 0 {
			add(s.NeedsAuth, "!", style.RoleWarn)
		}
		if len(runs) == 0 {
			return []mcpRun{{"0/" + strconv.Itoa(total), ""}}
		}
		return runs
	case 2:
		return []mcpRun{{strconv.Itoa(s.Connected) + "/" + strconv.Itoa(total), ""}}
	}
	return []mcpRun{{strconv.Itoa(s.Connected), ""}}
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
