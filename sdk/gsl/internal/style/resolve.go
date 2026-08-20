package style

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// knownNamedColors mirrors render.namedColor. Kept in sync manually; these
// are the eight conventional color names the style package documents.
var knownNamedColors = map[string]bool{
	"black":   true,
	"red":     true,
	"green":   true,
	"yellow":  true,
	"blue":    true,
	"magenta": true,
	"cyan":    true,
	"white":   true,
}

// validateUntrustedColor returns true when value is a valid color for an
// untrusted (non-config) source:
//   - a known named color ("blue", "cyan", …)
//   - a bare decimal ANSI-256 index in [0, 255]
//   - a well-formed truecolor fragment "38;2;r;g;b" or "48;2;r;g;b"
//   - the special value "default"
//
// Any other ';'-bearing string, control character, ESC byte, or
// out-of-range index returns false. This mirrors render.colorCode(v, _, false)
// but lives in the style package to avoid an import cycle.
func validateUntrustedColor(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return true // empty / "default" → no escape emitted; treated as valid
	}
	if strings.ContainsRune(value, '\x1b') {
		return false
	}
	if strings.Contains(value, ";") {
		return validateTruecolorFrag(value)
	}
	if knownNamedColors[value] {
		return true
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	return n >= 0 && n <= 255
}

// validateTruecolorFrag returns true for "38;2;r;g;b" or "48;2;r;g;b" with
// each component in [0, 255]. Mirrors render.validateTruecolor.
func validateTruecolorFrag(s string) bool {
	parts := strings.SplitN(s, ";", 6)
	if len(parts) != 5 {
		return false
	}
	if parts[0] != "38" && parts[0] != "48" {
		return false
	}
	if parts[1] != "2" {
		return false
	}
	for _, p := range parts[2:] {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// ResolveConfig is like Resolve but accepts user style overrides as raw
// map[string]any (the shape config.Styles carries after JSON decode). This
// allows the merge to be fill-presence-aware: a user entry that does NOT
// contain a "fill" key will NOT override the builtin's fill value, avoiding the
// problem where a user's `{"separator":"thin"}` silently sets fill=false on a
// builtin that has fill=true.
//
// autoPalette is the palette name returned by internal/theme.Resolve (e.g.
// "dark", "light", "dark-daltonism", "dark8"). When non-empty, its color
// values are merged into the resulting Style.Theme for the five segment keys
// that the user did NOT set in their config. User-set keys always win.
// Auto-merged values are validated through the untrusted colorCode path
// (trusted=false) before admission — our own palette constants will pass;
// this enforces the "non-config source is validated" invariant (SEC-4).
// Pass "" to skip auto-palette merging (backward-compatible, same as before).
//
// This is the variant that cmd/ should call when wiring config.Styles.
func ResolveConfig(w io.Writer, styleName string, rawUserStyles map[string]map[string]any, forceASCII bool, autoPalette string) Style {
	// Convert raw entries to Style, preserving fill-presence information.
	typed := make(map[string]Style, len(rawUserStyles))
	hasFill := make(map[string]bool, len(rawUserStyles))
	for k, raw := range rawUserStyles {
		s := rawToStyle(raw)
		typed[k] = s
		// Finding #7: only treat "fill" as an override when it is a valid JSON
		// bool. A "fill" key whose value is the wrong type (e.g. the string
		// "yes") must NOT trigger the override — otherwise rawToStyle's
		// zero-valued Fill (false) would silently clobber a builtin's
		// fill:true. Presence alone is insufficient; the value must be usable.
		fillVal, fillKeyPresent := raw["fill"]
		_, fillIsBool := fillVal.(bool)
		hasFill[k] = fillKeyPresent && fillIsBool
	}

	// ── 1. Look up built-in ──────────────────────────────────────────────────
	base, found := Builtin(styleName)
	if !found {
		fmt.Fprintf(w, "gsl: style %q is unknown; falling back to \"powerline\"\n", styleName)
		base, _ = Builtin("powerline")
	}

	// ── 2. Deep-merge user override (if any), respecting fill presence ───────
	if user, ok := typed[styleName]; ok {
		base = mergeIntoWithFillFlag(base, user, hasFill[styleName])
	}

	// ── 2b. Auto-palette merge (F2) ──────────────────────────────────────────
	// Apply auto-theme colors ONLY for segment keys the user did NOT set.
	// User keys always win. Values are validated through the untrusted path
	// before admission (SEC-4): our own palette constants pass; injection
	// attempts from a compromised palette are rejected.
	if autoPalette != "" {
		if paletteColors, ok := Palette(autoPalette); ok {
			// Collect the set of theme keys the user explicitly set.
			userTheme := map[string]bool{}
			if user, ok := typed[styleName]; ok {
				for k := range user.Theme {
					userTheme[k] = true
				}
			}
			if base.Theme == nil {
				base.Theme = make(map[string]string, len(paletteColors))
			}
			for _, key := range segmentColorKeys {
				if userTheme[key] {
					// User explicitly set this key → do not overwrite.
					continue
				}
				v, hasPalette := paletteColors[key]
				if !hasPalette {
					continue
				}
				// Validate through the untrusted path before admitting.
				if !validateUntrustedColor(v) {
					continue
				}
				base.Theme[key] = v
			}
		}
	}

	// ── 3. ASCII fallback ─────────────────────────────────────────────────────
	if forceASCII || base.Glyphs == "ascii" {
		base.Glyphs = "ascii"
		merged := make(map[string]string, len(asciiIcons))
		for k, v := range asciiIcons {
			merged[k] = v
		}
		if user, ok := typed[styleName]; ok {
			for k, v := range user.Icons {
				merged[k] = v
			}
		}
		base.Icons = merged
	}

	return base
}

// rawToStyle converts a raw map[string]any (from JSON decode) to a Style.
// Unknown keys are silently ignored. Missing keys yield zero values.
func rawToStyle(raw map[string]any) Style {
	s := Style{}
	if v, ok := raw["separator"]; ok {
		if sv, ok := v.(string); ok {
			s.Separator = sv
		}
	}
	if v, ok := raw["fill"]; ok {
		if bv, ok := v.(bool); ok {
			s.Fill = bv
		}
	}
	if v, ok := raw["glyphs"]; ok {
		if sv, ok := v.(string); ok {
			s.Glyphs = sv
		}
	}
	if v, ok := raw["icons"]; ok {
		if mv, ok := v.(map[string]any); ok {
			s.Icons = make(map[string]string, len(mv))
			for k, iv := range mv {
				if sv, ok := iv.(string); ok {
					s.Icons[k] = sv
				}
			}
		}
	}
	if v, ok := raw["theme"]; ok {
		if mv, ok := v.(map[string]any); ok {
			s.Theme = make(map[string]string, len(mv))
			for k, tv := range mv {
				if sv, ok := tv.(string); ok {
					s.Theme[k] = sv
				}
			}
		}
	}
	return s
}

// mergeIntoWithFillFlag is like mergeInto but only applies user.Fill when
// applyFill is true (i.e. the raw JSON entry contained a "fill" key).
func mergeIntoWithFillFlag(base Style, user Style, applyFill bool) Style {
	if user.Separator != "" {
		base.Separator = user.Separator
	}
	if applyFill {
		base.Fill = user.Fill
	}
	if user.Glyphs != "" {
		base.Glyphs = user.Glyphs
	}
	if len(user.Icons) > 0 {
		if base.Icons == nil {
			base.Icons = make(map[string]string, len(user.Icons))
		}
		for k, v := range user.Icons {
			base.Icons[k] = v
		}
	}
	if len(user.Theme) > 0 {
		if base.Theme == nil {
			base.Theme = make(map[string]string, len(user.Theme))
		}
		for k, v := range user.Theme {
			base.Theme[k] = v
		}
	}
	return base
}

// Resolve returns the effective Style for styleName by:
//
//  1. Looking up the built-in named by styleName. If the name is unknown the
//     function falls back to "powerline" AND writes a warning to w (never panics
//     or crashes — unknown style ≠ fatal error).
//
//  2. Deep-merging any same-named entry from userStyles OVER the built-in:
//     - Scalar fields (Separator, Fill, Glyphs) from the user entry overwrite the
//     built-in only when the user entry carries a non-zero value for that field.
//     (Empty string "" or false bool are treated as "not set" for scalars.)
//     - Icons and Theme maps are merged key-by-key: user keys win, unspecified
//     keys are inherited from the built-in.
//     - A brand-new style name (not a built-in) in userStyles resolves to the
//     user entry deep-merged over the powerline base.
//
//  3. If the resolved Glyphs is "ascii" OR forceASCII is true, the effective
//     Icons map is replaced by the ASCII fallback table (see builtins.go).
//     This guarantees safe output regardless of the terminal's font support.
//
// The w parameter receives diagnostic warnings; pass os.Stderr for production
// use or a bytes.Buffer / strings.Builder in tests.
//
// Resolve never returns a zero-valued Style; a valid style is always produced.
func Resolve(w io.Writer, styleName string, userStyles map[string]Style, forceASCII bool) Style {
	// ── 1. Look up built-in ──────────────────────────────────────────────────
	base, found := Builtin(styleName)
	if !found {
		fmt.Fprintf(w, "gsl: style %q is unknown; falling back to \"powerline\"\n", styleName)
		base, _ = Builtin("powerline")
	}

	// ── 2. Deep-merge user override (if any) ─────────────────────────────────
	// An unknown style name with no user entry needs no merge: base is already
	// powerline from the fallback above.
	if user, ok := userStyles[styleName]; ok {
		base = mergeInto(base, user)
	}

	// Special case: brand-new style name (not a built-in) with a user entry.
	// Handled above only if the name is not found as a built-in, but if the
	// name IS found the merge already happened. If !found AND there WAS a user
	// entry, the merge above covered it. No extra logic needed.

	// ── 3. ASCII fallback ─────────────────────────────────────────────────────
	if forceASCII || base.Glyphs == "ascii" {
		base.Glyphs = "ascii"
		merged := make(map[string]string, len(asciiIcons))
		for k, v := range asciiIcons {
			merged[k] = v
		}
		// Allow the user to override individual ASCII icons (e.g. preferred
		// brackets) even in ascii mode.
		if user, ok := userStyles[styleName]; ok {
			for k, v := range user.Icons {
				merged[k] = v
			}
		}
		base.Icons = merged
	}

	return base
}

// mergeInto applies the non-zero fields of user on top of base and returns the
// merged Style. base is already a defensive copy so we modify it in place.
func mergeInto(base Style, user Style) Style {
	// Scalar fields: user wins only when explicitly set (non-zero).
	if user.Separator != "" {
		base.Separator = user.Separator
	}
	// Finding #6: Fill is a bool, and the typed userStyles map carries no
	// fill-presence information, so we cannot distinguish "user set fill:false"
	// from "user omitted fill". To honor this function's documented contract
	// — scalar fields override "only when non-zero", and a false bool is
	// "treated as not set" — we apply user.Fill only when it is true. This
	// mirrors the presence-aware behavior of mergeIntoWithFillFlag (used by
	// ResolveConfig) so a user override of only {Separator:"thin"} no longer
	// silently clobbers a builtin's Fill:true. Callers that need to force
	// fill:false should use ResolveConfig, which is fill-presence-aware.
	if user.Fill {
		base.Fill = true
	}

	if user.Glyphs != "" {
		base.Glyphs = user.Glyphs
	}

	// Maps: key-by-key merge.
	if len(user.Icons) > 0 {
		if base.Icons == nil {
			base.Icons = make(map[string]string, len(user.Icons))
		}
		for k, v := range user.Icons {
			base.Icons[k] = v
		}
	}
	if len(user.Theme) > 0 {
		if base.Theme == nil {
			base.Theme = make(map[string]string, len(user.Theme))
		}
		for k, v := range user.Theme {
			base.Theme[k] = v
		}
	}
	return base
}
