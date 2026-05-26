package style

import (
	"fmt"
	"io"
)

// ResolveConfig is like Resolve but accepts user style overrides as raw
// map[string]any (the shape config.Styles carries after JSON decode). This
// allows the merge to be fill-presence-aware: a user entry that does NOT
// contain a "fill" key will NOT override the builtin's fill value, avoiding the
// problem where a user's `{"separator":"thin"}` silently sets fill=false on a
// builtin that has fill=true.
//
// This is the variant that cmd/ should call when wiring config.Styles.
func ResolveConfig(w io.Writer, styleName string, rawUserStyles map[string]map[string]any, forceASCII bool) Style {
	// Convert raw entries to Style, preserving fill-presence information.
	typed := make(map[string]Style, len(rawUserStyles))
	hasFill := make(map[string]bool, len(rawUserStyles))
	for k, raw := range rawUserStyles {
		s := rawToStyle(raw)
		typed[k] = s
		_, fillPresent := raw["fill"]
		hasFill[k] = fillPresent
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
//   - Scalar fields (Separator, Fill, Glyphs) from the user entry overwrite the
//     built-in only when the user entry carries a non-zero value for that field.
//     (Empty string "" or false bool are treated as "not set" for scalars.)
//   - Icons and Theme maps are merged key-by-key: user keys win, unspecified
//     keys are inherited from the built-in.
//   - A brand-new style name (not a built-in) in userStyles resolves to the
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
	if user, ok := userStyles[styleName]; ok {
		base = mergeInto(base, user)
	} else if !found {
		// Unknown style name AND no user entry → base is already powerline;
		// nothing extra to merge.
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
	// Fill is a bool; false could legitimately mean "I want no fill", so we
	// cannot use the zero-value heuristic. We apply user.Fill unconditionally
	// when the user provides ANY non-zero field, to avoid silently ignoring a
	// deliberate Fill=false. The simpler and more predictable rule: always
	// apply the user's Fill value so that an explicit override always wins.
	base.Fill = user.Fill

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
