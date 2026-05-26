package style

import (
	"fmt"
	"io"
)

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
