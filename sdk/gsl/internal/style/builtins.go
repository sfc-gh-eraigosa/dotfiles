package style

import "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"

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
	nfSepRight     = ""      // Powerline right-filled separator
	nfSepRightThin = ""      // Powerline right-thin sub-separator
	nfFolder       = ""      // Folder icon (dirgit)
	nfBranch       = ""      // Branch icon
	nfRepoRoot     = ""      // Repo-root icon (main worktree)
	nfWorktree     = ""      // Worktree icon (linked worktree)
	nfStaged       = ""      // Staged-changes icon
	nfUnstaged     = ""      // Unstaged-changes icon (warning)
	nfUntracked    = ""      // Untracked-files icon (question mark)
	nfStash        = ""      // Stash icon
	nfAhead        = ""      // Ahead-of-remote icon
	nfBehind       = ""      // Behind-remote icon
	nfAI           = ""      // AI / rocket icon
	nfMCP          = ""      // MCP / plug icon
	nfTime         = ""      // Clock icon
	nfContext      = ""      // Context-window / circuit icon
	nfVideo        = "\uf108" // Desktop / monitor icon (video)
	nfWTCount      = ""      // Worktree-count prefix icon
)

// powerlineStyle is the default built-in style. It uses Powerline-style filled
// separators, Nerd Font glyphs, and colored segment backgrounds. The repo_root
// segment is tinted blue; the repo_worktree segment is tinted magenta.
var powerlineStyle = Style{
	Separator: "powerline",
	Fill:      true,
	Glyphs:    "nerdfont",
	Icons: map[string]string{
		"host":           "💻",
		"host_macos":     "🍎",
		"host_wsl":       "🪟",
		"host_rpi":       "🍓",
		"host_jetson":    "🤖",
		"host_dgx":       "🧠",
		"host_container": "📦",
		"dirgit":         nfFolder,
		"repo_root":      nfRepoRoot,
		"repo_worktree":  nfWorktree,
		"worktree_count": nfWTCount,
		"ai":             nfAI,
		"mcp":            nfMCP,
		"time":           nfTime,
		"video":          nfVideo,
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
		"fg":     "default",
		"bg":     "default",
		"accent": "white",
		// repo_root, repo_worktree, ai, dirgit, time, video, host: the
		// shared dark palette, filled in by init (see palettes).
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
		"host":           "💻",
		"host_macos":     "🍎",
		"host_wsl":       "🪟",
		"host_rpi":       "🍓",
		"host_jetson":    "🤖",
		"host_dgx":       "🧠",
		"host_container": "📦",
		"dirgit":         "📁",
		"repo_root":      "🏠",
		"repo_worktree":  "🌳",
		"worktree_count": "⑂",
		"ai":             "🤖",
		"mcp":            "🔌",
		"time":           "⏰",
		"video":          "🎮",
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
		"fg":     "default",
		"bg":     "default",
		"accent": "default",
		// segment colors: the shared dark palette, filled in by init.
	},
}

// asciiIcons is the fallback icon table applied whenever Glyphs is "ascii"
// (either by the named style or by the forceASCII flag in Resolve). All values
// use plain printable ASCII so they are safe in any terminal.
var asciiIcons = map[string]string{
	"host":           "[host]",
	"dirgit":         "[dir]",
	"repo_root":      "[root]",
	"repo_worktree":  "[wt]",
	"worktree_count": "wt",
	"ai":             "[ai]",
	"mcp":            "[mcp]",
	"time":           "[time]",
	"video":          "[video]",
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

// segmentColorKeys lists the six theme keys that every palette must define so
// the emoji style (Fill:false) can apply fg-tint colors to every segment.
// See EMOJI-F2-01/F2-02 in the design doc.
var segmentColorKeys = []string{
	"repo_root",
	"repo_worktree",
	"ai",
	"dirgit",
	"time",
	"video",
	"host",
}

// palettes maps each segment color key to a role of the sdk's one color
// scheme (sdk/libs/tui/theme), so a palette name looks the same on the status
// line as in gff, fleet, mono and gdash. Built once from the library:
//
//	repo_root → Info     repo_worktree → Accent   ai   → Series[3]
//	dirgit    → OK       time          → Warn     host → Series[2]
//	video     → Crit
//
// Every index stays mid-luminance (TestPalette_MidLuminance), which the
// emoji style (Fill:false, color as fg tint only) needs on either background.
// dark8 is spelled with color names ("blue", …), so no 256-color escape is
// emitted and the terminal's own palette applies.
var palettes = buildPalettes()

// The built-in styles' segment colors are the dark palette, so a style with
// no auto-palette looks exactly like one resolved to "dark".
func init() {
	for _, st := range []*Style{&powerlineStyle, &emojiStyle} {
		for k, v := range palettes[theme.Dark] {
			st.Theme[k] = v
		}
	}
}

func buildPalettes() map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, name := range theme.Names() {
		r, _ := theme.Get(name)
		m := map[string]string{
			"repo_root":     r.Info,
			"repo_worktree": r.Accent,
			"ai":            r.SeriesColor(3),
			"dirgit":        r.OK,
			"time":          r.Warn,
			"video":         r.Crit,
			"host":          r.SeriesColor(2),
		}
		if name == theme.Dark8 {
			for k, v := range m {
				if n, ok := theme.BasicName(v); ok {
					m[k] = n
				}
			}
		}
		out[name] = m
	}
	return out
}

// Palette returns the color map for the named palette, and whether it was
// found. The returned map is a defensive copy.
//
// Callers use the returned map to merge auto-theme colors into a Style.Theme
// for the six segment keys that the user did not override.
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
