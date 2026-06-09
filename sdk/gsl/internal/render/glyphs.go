package render

import (
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// truecolorRe matches a well-formed truecolor SGR fragment: "38;2;r;g;b" or
// "48;2;r;g;b" where r, g, b are each in [0,255]. We validate component
// bounds manually after splitting rather than using a regexp, to keep the
// dependency surface minimal.
//
// validateTruecolor returns true when s is exactly "38;2;r;g;b" or
// "48;2;r;g;b" with all RGB components in [0, 255].
func validateTruecolor(s string) bool {
	parts := strings.SplitN(s, ";", 6)
	if len(parts) != 5 {
		return false
	}
	// Leading byte must be "38" or "48".
	if parts[0] != "38" && parts[0] != "48" {
		return false
	}
	// Second byte must be "2" (direct-color selector).
	if parts[1] != "2" {
		return false
	}
	// Remaining three parts must be decimal integers in [0, 255].
	for _, p := range parts[2:] {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

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
// trusted controls validation strictness — see colorCode for details.
//
// Accepted value forms (per the style package doc):
//   - named colour ("blue", "magenta", …) → 38;5;<index>
//   - decimal ANSI-256 index ("12", "201") → 38;5;<index>
//   - raw escape fragment ("38;5;12")      → used verbatim (trusted only)
//   - truecolor fragment ("38;2;r;g;b")    → always accepted when well-formed
func fgSeq(value string, trusted bool) string {
	code := colorCode(value, "38", trusted)
	if code == "" {
		return ""
	}
	return "\x1b[" + code + "m"
}

// bgSeq returns the ANSI background escape for a colour value, or "" when the
// value is empty or "default". Same value forms as fgSeq but emitted as a
// background (48;5;…) sequence.
//
// trusted controls validation strictness — see colorCode for details.
func bgSeq(value string, trusted bool) string {
	code := colorCode(value, "48", trusted)
	if code == "" {
		return ""
	}
	return "\x1b[" + code + "m"
}

// colorCode normalises a theme colour value into the numeric body of an ANSI
// SGR sequence for the given layer ("38" foreground / "48" background).
// Returns "" for empty/"default" values, and "" when an untrusted value fails
// validation (so no escape is emitted and the segment renders plain).
//
// trusted distinguishes two call paths:
//
//   - trusted == true: the value originates from the user's own config.json.
//     Behavior is fully backward-compatible: named colors, decimal ANSI-256
//     indices, and any ';'-bearing raw fragments are accepted verbatim.
//
//   - trusted == false: the value comes from an external source (host-tool
//     settings.json, env-derived palette). Only the following are accepted:
//     (1) a bare decimal ANSI-256 index in [0, 255],
//     (2) a well-formed truecolor fragment "38;2;r;g;b" or "48;2;r;g;b"
//     with r, g, b each in [0, 255], or
//     (3) a known named color from the namedColor map.
//     Any other ';'-bearing string, control character, ESC byte, or
//     out-of-range index is rejected and returns "".
//
// All current callers (paint via fgSeq/bgSeq from builtin/user styles) are
// trusted. Phase 4 will pass trusted=false for auto-theme/settings-derived
// color values.
func colorCode(value, layer string, trusted bool) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return ""
	}

	// Reject any value containing a raw ESC byte regardless of trust level.
	// (Trusted path still rejects ESC — the user's config.json is not expected
	// to contain literal control bytes, and permitting them would allow
	// escape-sequence injection even through config.json.)
	if strings.ContainsRune(value, '\x1b') {
		return ""
	}

	if strings.Contains(value, ";") {
		if trusted {
			// Trusted path: honour any ';'-bearing fragment verbatim (back-compat).
			return value
		}
		// Untrusted path: only well-formed truecolor sequences are accepted.
		if validateTruecolor(value) {
			return value
		}
		return ""
	}

	// Named colour → 256-index. Accepted on both paths.
	if idx, ok := namedColor[value]; ok {
		return layer + ";5;" + idx
	}

	// Decimal ANSI-256 index.
	n, err := strconv.Atoi(value)
	if err != nil {
		return ""
	}
	if !trusted && (n < 0 || n > 255) {
		// Untrusted path: reject out-of-range indices.
		return ""
	}
	return layer + ";5;" + value
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
		bg := bgSeq(color, true)
		if bg == "" {
			return text
		}
		fg := fgSeq(themeColor(st, "fg"), true)
		if fg == "" {
			// Default fg on a coloured block is hard to read; use white.
			fg = fgSeq("white", true)
		}
		return bg + fg + text + ansiReset
	}
	// No fill: tint the foreground with the segment's colour.
	fg := fgSeq(color, true)
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

// segmentBlock carries the raw (unpainted) text from a segment together with
// the theme colorKey the join layer should paint it with. The join layer owns
// all ANSI emission — segments never embed escape sequences.
type segmentBlock struct {
	text     string
	colorKey string
}

// join assembles the surviving segment blocks into the final status line.
//
// For powerline + Fill it emits:
//   - each block painted with its own bg+fg via paint()
//   - between adjacent blocks: a color-bridged chevron (bg=next color,
//     fg=prev color, glyph, reset)
//   - after the last block: a trailing fade chevron (fg=last color, glyph, reset)
//
// For thin/space/emoji (or when Fill is false) it falls through to the
// classic space-padded-glyph separator path, preserving existing behavior
// exactly — no bridged chevrons, just the old fg-tint paint per block.
func join(st style.Style, blocks []segmentBlock) string {
	if len(blocks) == 0 {
		return ""
	}

	if st.Separator == "powerline" && st.Fill {
		return joinPowerline(st, blocks)
	}

	// Classic path: paint each block with fg tint (or bg fill if Fill is true
	// but separator is not "powerline"), join with the style separator.
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, paint(st, b.colorKey, b.text))
	}
	return strings.Join(parts, separator(st))
}

// joinPowerline builds the powerline wall-to-wall ribbon with color-bridged
// chevrons. Each interior boundary carries bg=next,fg=prev; the trailing
// chevron fades fg=last to the terminal background.
func joinPowerline(st style.Style, blocks []segmentBlock) string {
	sepGlyph := glyph(st, "sep_right")
	if sepGlyph == "" {
		// No glyph available: fall back to classic path without bridges.
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			parts = append(parts, paint(st, b.colorKey, b.text))
		}
		return strings.Join(parts, separator(st))
	}

	var sb strings.Builder

	for i, b := range blocks {
		// Paint the block: bg=this color, fg=global "fg" (or white fallback).
		// paint() adds a trailing ansiReset — we need the raw sequences instead
		// so we can emit the bridge chevron with the correct context. We
		// reproduce paint()'s Fill:true logic inline here.
		color := themeColor(st, b.colorKey)
		bg := bgSeq(color, true)
		fg := fgSeq(themeColor(st, "fg"), true)
		if fg == "" {
			fg = fgSeq("white", true)
		}
		if bg != "" {
			sb.WriteString(bg)
			sb.WriteString(fg)
		}
		// Space pad before and after the text (mirrors the existing style).
		sb.WriteString(" ")
		sb.WriteString(b.text)
		sb.WriteString(" ")

		// Emit the bridge chevron.
		if i < len(blocks)-1 {
			// Interior chevron: bg=next block's color, fg=this block's color.
			nextColor := themeColor(st, blocks[i+1].colorKey)
			nextBG := bgSeq(nextColor, true)
			thisFG := fgSeq(color, true)
			if nextBG != "" {
				sb.WriteString(nextBG)
			}
			if thisFG != "" {
				sb.WriteString(thisFG)
			}
		} else {
			// Trailing chevron: reset bg (fade to terminal), fg=this block's color.
			thisFG := fgSeq(color, true)
			sb.WriteString(ansiReset)
			if thisFG != "" {
				sb.WriteString(thisFG)
			}
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
