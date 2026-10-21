package render

import (
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// ANSI control sequences. Kept here so every escape code in the render layer
// lives in one place.
const (
	ansiReset = "\x1b[0m"
)

// glyph returns the icon string for key from st.Icons. Resolve has already
// substituted the ASCII fallback table when Glyphs == "ascii" or forceASCII
// was set, so a simple map lookup is correct for every glyph mode. Missing
// keys yield "" so a segment that asks for an unknown glyph degrades to no
// glyph rather than crashing.
func glyph(st style.Style, key string) string {
	if st.Icons == nil {
		return ""
	}
	return st.Icons[key]
}

// colorMode is the mode the render paints in (ModeAuto paints as 256).
func colorMode(st style.Style) style.ColorMode {
	if st.Mode == style.ModeAuto {
		return style.Mode256
	}
	return st.Mode
}

// blockOpen is the escape that starts a block of key's colour, and therefore
// also what returns to it after a styled run inside it:
//   - Fill: the block's bg and its readable text (ResolveBlockColors — always
//     concrete, so a block can never be left unpainted);
//   - tint: the key's colour as foreground, or "" for none.
func blockOpen(st style.Style, key string) string {
	m := colorMode(st)
	if st.Fill {
		bg, fg := style.ResolveBlockColors(st, style.Role(key))
		return bg.SGR(true, m) + fg.SGR(false, m)
	}
	return style.RoleColor(st, style.Role(key)).SGR(false, m)
}

// paint wraps text — already carrying its spans — in the block colour for
// key, ending with a reset. Under ModeNever it is the plain text.
func paint(st style.Style, key, text string, spans []LinkSpan) string {
	if colorMode(st) == style.ModeNever {
		return text
	}
	open := blockOpen(st, key)
	runs := paintRuns(st, text, spans, open)
	if open == "" {
		if runs == text {
			return text
		}
		return runs + ansiReset
	}
	return open + runs + ansiReset
}

// separator returns the inter-segment separator string for the style.
//
//	"powerline" → the filled right-chevron glyph (Icons["sep_right"]).
//	"thin"      → the thin sub-separator glyph (Icons["sep_right_thin"]).
//	anything else (incl. "space") → a single space.
//
// A space is always padded around a glyph separator so adjacent segment text
// does not abut the chevron.
func separator(st style.Style) string {
	switch st.Separator {
	case "powerline":
		g := glyph(st, "sep_right")
		if g == "" {
			return " "
		}
		return " " + g + " "
	case "thin":
		g := glyph(st, "sep_right_thin")
		if g == "" {
			return " "
		}
		return " " + g + " "
	default: // "space" and any unknown value
		return " "
	}
}

// segmentBlock carries the raw (unpainted) text from a segment together with
// the theme colorKey the join layer should paint it with. The join layer owns
// all ANSI emission — segments never embed escape sequences.
type segmentBlock struct {
	text     string
	colorKey string
	// links are the clickable byte ranges of text, each with its URL. They are
	// deliberately NOT part of text: text is what the width machinery measures
	// and truncates, and a URL is zero display width. Keeping them apart is what
	// stops a long URL from being mistaken for content.
	links []LinkSpan
}

// paintRuns emits text with every valid span applied: a Role paints its run
// (Fill: a chip, the role as background with readable text; else a tint),
// a URL wraps it as an OSC 8 hyperlink, underlined unless st.Links ==
// "plain". After a styled run, restore (the block's own escape) puts the
// block back; with no block colour the foreground is reset to the default.
// ST (ESC \) terminates the OSC; term.StripANSI accepts it when measuring.
// Under ModeNever nothing is emitted: the text is returned as is.
func paintRuns(st style.Style, text string, spans []LinkSpan, restore string) string {
	m := colorMode(st)
	spans = validateSpans(len(text), spans)
	if len(spans) == 0 || m == style.ModeNever {
		return text
	}
	ul, ulOff := sgrUnderline, sgrNoUnderline
	if st.Links == "plain" {
		ul, ulOff = "", ""
	}
	if restore == "" {
		restore = "\x1b[39m" // default foreground
	}
	var sb strings.Builder
	pos := 0
	for _, sp := range spans {
		sb.WriteString(text[pos:sp.Start])
		run := text[sp.Start:sp.End]
		if sp.Role != "" {
			if c := style.RoleColor(st, style.Role(sp.Role)); !c.IsZero() {
				if st.Fill {
					run = c.SGR(true, m) + style.TextOn(c).SGR(false, m) + run + restore
				} else {
					run = c.SGR(false, m) + run + restore
				}
			}
		}
		if sp.URL != "" {
			run = "\x1b]8;;" + sp.URL + "\x1b\\" + ul + run + ulOff + "\x1b]8;;\x1b\\"
		}
		sb.WriteString(run)
		pos = sp.End
	}
	sb.WriteString(text[pos:])
	return sb.String()
}

func join(st style.Style, blocks []segmentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	if colorMode(st) != style.ModeNever && st.Separator == "powerline" && st.Fill && glyph(st, "sep_right") != "" {
		return joinPowerline(st, blocks)
	}
	// Classic path: paint each block (fill or tint), join with the separator.
	// Under ModeNever paint is the plain text, so the line has no escape.
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, paint(st, b.colorKey, b.text, b.links))
	}
	return strings.Join(parts, separator(st))
}

// joinPowerline builds the wall-to-wall ribbon with colour-bridged chevrons.
// Block and chevron ask the SAME resolver (ResolveBlockColors), so they can
// never disagree: each interior chevron is bg=next block, fg=this block; the
// trailing one fades fg=last block onto the terminal background.
func joinPowerline(st style.Style, blocks []segmentBlock) string {
	m := colorMode(st)
	sepGlyph := glyph(st, "sep_right")
	var sb strings.Builder
	for i, b := range blocks {
		bg, _ := style.ResolveBlockColors(st, style.Role(b.colorKey))
		open := blockOpen(st, b.colorKey)
		sb.WriteString(open)
		// Hyperlinks cover only their spans: the padding and the chevron are
		// shared decoration, so a link must not extend over them.
		sb.WriteString(" " + paintRuns(st, b.text, b.links, open) + " ")
		if i < len(blocks)-1 {
			next, _ := style.ResolveBlockColors(st, style.Role(blocks[i+1].colorKey))
			sb.WriteString(next.SGR(true, m) + bg.SGR(false, m))
		} else {
			sb.WriteString(ansiReset + bg.SGR(false, m))
		}
		sb.WriteString(sepGlyph)
		sb.WriteString(ansiReset)
	}
	return sb.String()
}

// countBadge renders a "<glyph><n>" badge, e.g. "+3" / "⇡2". The glyph is
// looked up by iconKey; when the icon is empty only the number is shown.
func countBadge(st style.Style, iconKey string, n int) string {
	g := glyph(st, iconKey)
	if g == "" {
		return strconv.Itoa(n)
	}
	return g + strconv.Itoa(n)
}
