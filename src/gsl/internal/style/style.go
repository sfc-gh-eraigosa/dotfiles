// Package style defines the Style type, built-in style presets, and the
// Resolve function that produces the effective Style for a given name.
//
// # Style
//
// A Style is a named bundle that controls how the status line is PRESENTED.
// It does NOT change which data segments are collected or their values; it
// only affects visual presentation (icons, separators, colors, fill).
//
// # Separator values
//
//   - "powerline" — uses Powerline/Nerd-Font curved-arrow separators (e.g.  / ).
//   - "thin"      — uses thin vertical-bar separators (e.g.  / ).
//   - "space"     — uses plain space-padding between segments.
//
// # Glyphs values
//
//   - "nerdfont" — expects a Nerd Font terminal; uses Nerd-Font codepoints.
//   - "emoji"    — uses Unicode emoji characters (no font dependency).
//   - "ascii"    — uses plain ASCII fallback strings; also activated when
//     [Resolve] receives forceASCII=true.
//
// # Icons keys (conventional — renderers may add more)
//
//	dirgit          current-directory glyph when inside a git repo
//	repo_root       main worktree glyph
//	repo_worktree   linked-worktree glyph
//	worktree_count  prefix glyph for the worktree-count badge
//	ai              AI-agent indicator glyph
//	mcp             MCP-server indicator glyph
//	time            clock glyph
//	branch          git branch glyph
//	ahead           commits-ahead glyph
//	behind          commits-behind glyph
//	staged          staged-changes glyph
//	unstaged        unstaged-changes glyph
//	untracked       untracked-files glyph
//	stash           stash-present glyph
//	context         context-window usage glyph
//
// # Theme keys (conventional)
//
//	fg           foreground (ANSI 256-color index or "default")
//	bg           background (ANSI 256-color index or "default")
//	accent       accent color used for separator glyphs
//	repo_root    segment background for main-worktree repo segment (default: blue → ANSI 12 / "blue")
//	repo_worktree segment background for linked-worktree repo segment (default: magenta → ANSI 13 / "magenta")
//	ai           segment background for the AI segment
//	dirgit       segment background for the dirgit segment
//	time         segment background for the time segment
//
// Color values may be:
//   - A decimal ANSI 256-color index string ("12", "201", …).
//   - A named color string ("blue", "magenta", "cyan", "green", "yellow",
//     "red", "white", "black", "default").
//   - An ANSI escape sequence fragment ("38;5;12") for direct embedding.
//
// Renderers are responsible for interpreting these values.
package style

// Style is the resolved visual configuration for the gsl status line.
type Style struct {
	// Separator controls the glyph used between segments.
	// Valid values: "powerline", "thin", "space".
	Separator string `json:"separator"`

	// Fill controls whether segments are drawn with a colored background block.
	// When false the segment text is printed without a colored background.
	Fill bool `json:"fill"`

	// Glyphs selects the icon repertoire.
	// Valid values: "nerdfont", "emoji", "ascii".
	Glyphs string `json:"glyphs"`

	// Icons maps logical icon keys to glyph strings. See package doc for the
	// conventional key set. Renderers look up keys at render time; missing
	// keys fall back to an empty string or the ASCII fallback table.
	Icons map[string]string `json:"icons,omitempty"`

	// Theme maps logical color-role keys to color values (ANSI index, named
	// color, or escape fragment). See package doc for the conventional key set.
	Theme map[string]string `json:"theme,omitempty"`
}
