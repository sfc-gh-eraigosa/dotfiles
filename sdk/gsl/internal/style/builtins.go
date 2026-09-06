package style

// Nerd Font glyph codepoints used by the powerline style.
//
// These are Unicode private-use-area codepoints provided by the
// Nerd Fonts project (https://www.nerdfonts.com/). A Nerd Font-patched
// terminal font is required to render them; otherwise the terminal shows
// replacement boxes or question marks.
//
// Codepoints used:
//
//	  — Powerline right-chevron filled  (segment separator →)
//	  — Powerline right-chevron thin    (sub-separator)
//	  — Nerd Font folder (dirgit)
//	  — Nerd Font branch symbol
//	  — Nerd Font git repo / root indicator
//	  — Nerd Font git branch (worktree)
//	  — Nerd Font plus-circle (staged)
//	  — Nerd Font warning (unstaged)
//	  — Nerd Font question-mark (untracked)
//	  — Nerd Font archive / stash
//	  — Nerd Font up-arrow (ahead)
//	  — Nerd Font down-arrow (behind)
//	  — Nerd Font rocket (ai)
//	  — Nerd Font plug (mcp)
//	  — Nerd Font clock (time)
//	  — Nerd Font circuit / context window
//	  — Nerd Font database (worktree_count prefix)

const (
	nfSepRight     = "" // Powerline right-filled separator
	nfSepRightThin = "" // Powerline right-thin sub-separator
	nfFolder       = "" // Folder icon (dirgit)
	nfBranch       = "" // Branch icon
	nfRepoRoot     = "" // Repo-root icon (main worktree)
	nfWorktree     = "" // Worktree icon (linked worktree)
	nfStaged       = "" // Staged-changes icon
	nfUnstaged     = "" // Unstaged-changes icon (warning)
	nfUntracked    = "" // Untracked-files icon (question mark)
	nfStash        = "" // Stash icon
	nfAhead        = "" // Ahead-of-remote icon
	nfBehind       = "" // Behind-remote icon
	nfAI           = "" // AI / rocket icon
	nfMCP          = "" // MCP / plug icon
	nfTime         = "" // Clock icon
	nfContext      = "" // Context-window / circuit icon
	nfWTCount      = "" // Worktree-count prefix icon
)

// powerlineStyle is the default built-in style. It uses Powerline-style filled
// separators, Nerd Font glyphs, and colored segment backgrounds. The repo_root
// segment is tinted blue; the repo_worktree segment is tinted magenta.
var powerlineStyle = Style{
	Separator: "powerline",
	Fill:      true,
	Glyphs:    "nerdfont",
	Icons: map[string]string{
		"dirgit":         nfFolder,
		"repo_root":      nfRepoRoot,
		"repo_worktree":  nfWorktree,
		"worktree_count": nfWTCount,
		"ai":             nfAI,
		"mcp":            nfMCP,
		"time":           nfTime,
		"branch":         nfBranch,
		"ahead":          nfAhead,
		"behind":         nfBehind,
		"staged":         nfStaged,
		"unstaged":       nfUnstaged,
		"untracked":      nfUntracked,
		"stash":          nfStash,
		"context":        nfContext,
		// separator glyphs exposed so renderers can pull from Icons if needed
		"sep_right":      nfSepRight,
		"sep_right_thin": nfSepRightThin,
	},
	Theme: map[string]string{
		"fg":            "default",
		"bg":            "default",
		"accent":        "white",
		"repo_root":     "blue",    // main worktree — blue tint
		"repo_worktree": "magenta", // linked worktree — magenta tint
		"ai":            "cyan",
		"dirgit":        "green",
		"time":          "yellow",
	},
}

// emojiStyle is an airy alternative that requires no special font. It uses
// Unicode emoji for icons, thin bar separators, and no filled background
// blocks, making it readable in any terminal.
var emojiStyle = Style{
	Separator: "thin",
	Fill:      false,
	Glyphs:    "emoji",
	Icons: map[string]string{
		"dirgit":         "📁",
		"repo_root":      "🏠",
		"repo_worktree":  "🌳",
		"worktree_count": "⑂",
		"ai":             "🤖",
		"mcp":            "🔌",
		"time":           "⏰",
		"branch":         "🌿",
		"ahead":          "⬆",
		"behind":         "⬇",
		"staged":         "✚",
		"unstaged":       "✎",
		"untracked":      "✦",
		"stash":          "📦",
		"context":        "🧠",
		"sep_right":      "|",
		"sep_right_thin": "·",
	},
	Theme: map[string]string{
		"fg":            "default",
		"bg":            "default",
		"accent":        "default",
		"repo_root":     "blue",
		"repo_worktree": "magenta",
		"ai":            "cyan",
		"dirgit":        "green",
		"time":          "yellow",
	},
}

// asciiIcons is the fallback icon table applied whenever Glyphs is "ascii"
// (either by the named style or by the forceASCII flag in Resolve). All values
// use plain printable ASCII so they are safe in any terminal.
var asciiIcons = map[string]string{
	"dirgit":         "[dir]",
	"repo_root":      "[root]",
	"repo_worktree":  "[wt]",
	"worktree_count": "wt",
	"ai":             "[ai]",
	"mcp":            "[mcp]",
	"time":           "[time]",
	"branch":         "br:",
	"ahead":          "+",
	"behind":         "-",
	"staged":         "*",
	"unstaged":       "!",
	"untracked":      "?",
	"stash":          "$",
	"context":        "[ctx]",
	"sep_right":      "|",
	"sep_right_thin": ":",
}

// builtins is the registry of compiled-in styles, keyed by style name.
var builtins = map[string]Style{
	"powerline": powerlineStyle,
	"emoji":     emojiStyle,
}

// segmentColorKeys lists the five theme keys that every palette must define so
// the emoji style (Fill:false) can apply fg-tint colors to every segment.
// See EMOJI-F2-01/F2-02 in the design doc.
var segmentColorKeys = []string{
	"repo_root",
	"repo_worktree",
	"ai",
	"dirgit",
	"time",
}

// palettes holds the compiled-in color palettes keyed by palette name.
//
// Each palette defines at minimum all five segment color keys
// (repo_root, repo_worktree, ai, dirgit, time). Indices in the
// non-default palettes are chosen to be mid-luminance (approximate
// luminance 30–220 out of 255) so they are legible as bare foreground
// color on both light and dark terminal backgrounds. This is required
// because the emoji style (Fill:false) renders color as fg-tint only —
// there is no background block for contrast.
//
// Palette selection:
//
//   - "dark"           — default; uses ANSI named-color strings (system 8)
//     that map to the terminal's own dark-palette colors (green, blue, …).
//   - "light"          — for light-background terminals; uses ANSI-256
//     indices with lower luminance (darker shades) for fg readability.
//   - "dark-daltonism" — red-green colorblind friendly; avoids red/green;
//     uses blue/orange/teal ANSI-256 indices.
//   - "dark8"          — 8-color terminals; uses named-color strings so the
//     terminal's own palette applies (no 256-color escapes emitted).
var palettes = map[string]map[string]string{
	// "dark" is the default palette — named colors (system 8) matching the
	// existing powerline/emoji built-in style theme values.
	"dark": {
		"repo_root":     "blue",    // ANSI 4 / system blue
		"repo_worktree": "magenta", // ANSI 5 / system magenta
		"ai":            "cyan",    // ANSI 6 / system cyan
		"dirgit":        "green",   // ANSI 2 / system green
		"time":          "yellow",  // ANSI 3 / system yellow
	},
	// "light" — darker ANSI-256 shades for readability on light backgrounds.
	// Each index is mid-luminance (~44–138/255) and legible on dark too.
	//   repo_root     26: (r=0,g=1,b=4) medium-dark blue           lum~83
	//   repo_worktree 92: (r=2,g=0,b=4) medium purple              lum~44
	//   ai            37: (r=0,g=3,b=3) medium teal/cyan           lum~138
	//   dirgit        34: (r=0,g=3,b=0) medium green               lum~125
	//   time         136: (r=3,g=2,b=0) medium amber/brown         lum~134
	"light": {
		"repo_root":     "26",
		"repo_worktree": "92",
		"ai":            "37",
		"dirgit":        "34",
		"time":          "136",
	},
	// "dark-daltonism" — red-green colorblind friendly; avoids red/green
	// for segment identity. Uses blue/orange/teal ANSI-256 indices.
	//   repo_root     69: (r=1,g=2,b=5) medium blue-violet         lum~135
	//   repo_worktree 208: (r=5,g=2,b=0) medium orange             lum~151
	//   ai            44: (r=0,g=4,b=4) medium cyan                lum~169
	//   dirgit        73: (r=1,g=3,b=3) medium teal                lum~158
	//   time         178: (r=4,g=3,b=0) medium golden amber        lum~171
	"dark-daltonism": {
		"repo_root":     "69",
		"repo_worktree": "208",
		"ai":            "44",
		"dirgit":        "73",
		"time":          "178",
	},
	// "dark8" — 8-color terminals; named-color strings so no 256-color
	// escapes are emitted; the terminal's own palette applies.
	"dark8": {
		"repo_root":     "blue",
		"repo_worktree": "magenta",
		"ai":            "cyan",
		"dirgit":        "green",
		"time":          "yellow",
	},
}

// Palette returns the color map for the named palette, and whether it was
// found. The returned map is a defensive copy.
//
// Callers use the returned map to merge auto-theme colors into a Style.Theme
// for the five segment keys that the user did not override.
func Palette(name string) (map[string]string, bool) {
	p, ok := palettes[name]
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out, true
}

// SegmentColorKeys returns the canonical list of theme keys that every palette
// must define — one per segment type. Used by tests to verify palette
// completeness and by ResolveConfig to drive the auto-theme merge.
func SegmentColorKeys() []string {
	out := make([]string, len(segmentColorKeys))
	copy(out, segmentColorKeys)
	return out
}

// Builtin returns the built-in style with the given name and whether it was
// found. The returned Style is a defensive copy — callers may modify it safely.
func Builtin(name string) (Style, bool) {
	s, ok := builtins[name]
	if !ok {
		return Style{}, false
	}
	return copyStyle(s), ok
}

// Builtins returns a map of all compiled-in styles. Each value is a defensive
// copy — callers may modify entries without affecting the registry.
func Builtins() map[string]Style {
	out := make(map[string]Style, len(builtins))
	for k, v := range builtins {
		out[k] = copyStyle(v)
	}
	return out
}

// copyStyle returns a deep copy of s so callers cannot mutate the built-in
// registry values via map-reference aliasing.
func copyStyle(s Style) Style {
	out := Style{
		Separator: s.Separator,
		Fill:      s.Fill,
		Glyphs:    s.Glyphs,
	}
	if s.Icons != nil {
		out.Icons = make(map[string]string, len(s.Icons))
		for k, v := range s.Icons {
			out.Icons[k] = v
		}
	}
	if s.Theme != nil {
		out.Theme = make(map[string]string, len(s.Theme))
		for k, v := range s.Theme {
			out.Theme[k] = v
		}
	}
	return out
}
