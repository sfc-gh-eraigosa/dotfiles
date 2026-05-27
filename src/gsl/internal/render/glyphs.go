package render

import (
	"strconv"
	"strings"

	"github.com/wenlock/dotfiles/gsl/internal/style"
)

// ANSI control sequences. Kept here so every escape code in the render layer
// lives in one place.
const (
	ansiReset = "\x1b[0m"
)

// namedFG maps the conventional colour names the style package documents to
// their ANSI 256-colour indices. Used for both foreground (38;5;N) and as the
// background source when Fill is enabled (48;5;N).
var namedColor = map[string]string{
	"black":   "0",
	"red":     "1",
	"green":   "2",
	"yellow":  "3",
	"blue":    "4",
	"magenta": "5",
	"cyan":    "6",
	"white":   "7",
}

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

// themeColor returns the raw theme value for key (e.g. "blue", "12",
// "38;5;12"), or "" when absent.
func themeColor(st style.Style, key string) string {
	if st.Theme == nil {
		return ""
	}
	return st.Theme[key]
}

// fgSeq returns the ANSI foreground escape for a colour value, or "" when the
// value is empty or "default" (meaning "leave the terminal default").
//
// Accepted value forms (per the style package doc):
//   - named colour ("blue", "magenta", …) → 38;5;<index>
//   - decimal ANSI-256 index ("12", "201") → 38;5;<index>
//   - raw escape fragment ("38;5;12")      → used verbatim
func fgSeq(value string) string {
	code := colorCode(value, "38")
	if code == "" {
		return ""
	}
	return "\x1b[" + code + "m"
}

// bgSeq returns the ANSI background escape for a colour value, or "" when the
// value is empty or "default". Same value forms as fgSeq but emitted as a
// background (48;5;…) sequence.
func bgSeq(value string) string {
	code := colorCode(value, "48")
	if code == "" {
		return ""
	}
	return "\x1b[" + code + "m"
}

// colorCode normalises a theme colour value into the numeric body of an ANSI
// SGR sequence for the given layer ("38" foreground / "48" background).
// Returns "" for empty/"default" values.
func colorCode(value, layer string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return ""
	}
	// Raw escape fragment already containing a ';' (e.g. "38;5;12"): the caller
	// asked for an explicit sequence — honour it verbatim, retargeting the
	// leading 38/48 to the requested layer when it differs is intentionally NOT
	// done; the value is used as-is.
	if strings.Contains(value, ";") {
		return value
	}
	// Named colour → 256-index.
	if idx, ok := namedColor[value]; ok {
		return layer + ";5;" + idx
	}
	// Decimal ANSI-256 index.
	if _, err := strconv.Atoi(value); err == nil {
		return layer + ";5;" + value
	}
	return ""
}

// paint wraps text in the segment's theme colours.
//
// When st.Fill is true the segment background block is drawn using the theme
// key bgKey (e.g. "repo_root", "dirgit"); foreground uses the theme "fg" value
// (or white for contrast when fg is "default"). When Fill is false only the
// foreground colour from bgKey's value is applied (so e.g. a worktree segment
// still tints magenta) — this gives the emoji/thin style colour without a
// background block. An empty/missing colour yields plain text.
//
// paint is a no-op (returns text unchanged) when no colour applies, so ASCII /
// no-theme styles produce clean, escape-free output.
func paint(st style.Style, bgKey, text string) string {
	color := themeColor(st, bgKey)
	if st.Fill {
		bg := bgSeq(color)
		if bg == "" {
			return text
		}
		fg := fgSeq(themeColor(st, "fg"))
		if fg == "" {
			// Default fg on a coloured block is hard to read; use white.
			fg = fgSeq("white")
		}
		return bg + fg + text + ansiReset
	}
	// No fill: tint the foreground with the segment's colour.
	fg := fgSeq(color)
	if fg == "" {
		return text
	}
	return fg + text + ansiReset
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

// join assembles the surviving segment texts into the final status line using
// the style's separator. Empty segments are already filtered by the caller.
func join(st style.Style, parts []string) string {
	return strings.Join(parts, separator(st))
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
