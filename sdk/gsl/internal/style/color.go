package style

// color.go — the FROZEN colour contract (gsl-ultra, plan §3).
//
// # The bug this exists to make impossible
//
// Colour is currently resolved in TWO places that disagree:
//
//   - paint()          (render/glyphs.go) — when the colour resolves to "",
//     returns the text UNPAINTED while its neighbours are painted.
//   - joinPowerline()  (render/glyphs.go) — nests the fg emit inside
//     `if bg != ""` but emits the chevron UNCONDITIONALLY.
//
// So a theme value that resolves to empty — a literal "default", a hex value
// (unsupported today, silently dropped), or an unknown role key — yields an
// unpainted block, a white wedge, and a stray trailing triangle.
//
// Separately, joinPowerline hardcodes WHITE as the foreground of every filled
// block regardless of its background. Against the builtin "time": "yellow",
// that is a contrast ratio of ~1.35:1 — effectively illegible.
//
// The fix is structural, not a patch: ONE resolver, consumed by BOTH painters,
// that ALWAYS returns a concrete (bg, fg) pair. A block and its chevron cannot
// disagree if they ask the same function.

// RGB is a resolved, concrete 24-bit colour. There is no "empty" RGB — that is
// the point. Every role resolves to a real colour; the capability ladder
// decides how it is EMITTED (truecolor / 256 / 16 / not at all), never whether
// it exists.
type RGB struct {
	R, G, B uint8
}

// Role names a semantic colour slot. Roles are the vocabulary the render layer
// and the TUI colour editor both speak.
//
// Segment roles map to the existing theme keys; STATE roles are new and are
// what makes the MCP breakdown legible (connected=ok, failed=err,
// needs-auth=warn).
type Role string

const (
	// Segment roles (existing theme keys).
	RoleDirGit       Role = "dirgit"
	RoleRepoRoot     Role = "repo_root"
	RoleRepoWorktree Role = "repo_worktree"
	RoleAI           Role = "ai"
	RoleTime         Role = "time"

	// State roles (new — consumed by the MCP breakdown and git-dirty badges).
	RoleOK    Role = "ok"    // healthy / connected
	RoleWarn  Role = "warn"  // actionable by the user (needs auth, behind)
	RoleErr   Role = "err"   // broken (failed to connect, conflicts)
	RoleMuted Role = "muted" // de-emphasized detail
)

// ColorMode is the terminal-capability ladder. Resolution is by capability, not
// by guess: NO_COLOR and TERM=dumb force ModeNever; COLORTERM selects the rung.
type ColorMode int

const (
	ModeAuto      ColorMode = iota // detect from the environment
	ModeNever                      // emit no escapes at all (NO_COLOR, TERM=dumb)
	Mode16                         // basic ANSI
	Mode256                        // ANSI 256-colour
	ModeTruecolor                  // 24-bit
)

// resolveBlockColors returns the concrete (bg, fg) pair for a role.
//
// CONTRACT — the invariant every caller relies on:
//
//	It ALWAYS returns a usable pair. There is no failure mode, no empty value,
//	no "" sentinel. An unknown role, an unparseable value, and a missing key
//	all resolve to a defined fallback rather than to nothing.
//
// Resolution:
//  1. The user's explicit config for the role ALWAYS wins (bg and/or fg).
//  2. A value may be a named colour, a decimal ANSI-256 index, or a hex
//     "#rrggbb" (hex is NEW — today it is silently dropped).
//  3. When only a bg is known, fg is COMPUTED for contrast: pick whichever of
//     the palette's light/dark foregrounds yields the higher WCAG contrast
//     ratio against bg. This is what kills white-on-yellow.
//
// Downsampling to the terminal's ColorMode happens at EMIT time, not here —
// this function always thinks in 24-bit truth.
func resolveBlockColors(st Style, role Role) (bg, fg RGB) {
	// Implemented in the `style` leaf.
	return RGB{}, RGB{}
}

// ResolveBlockColors is the exported form consumed by the render layer and the
// TUI. Same contract as resolveBlockColors.
func ResolveBlockColors(st Style, role Role) (bg, fg RGB) {
	return resolveBlockColors(st, role)
}

// ContrastRatio returns the WCAG 2.x contrast ratio between two colours, in
// [1, 21]. Used both to CHOOSE a contrast-safe foreground and to GATE the
// builtin palettes in test (every palette × role must clear 4.5:1).
func ContrastRatio(a, b RGB) float64 {
	// Implemented in the `style` leaf.
	return 0
}
