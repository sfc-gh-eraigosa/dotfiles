package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// color.go — the colour engine (gsl-ultra plan F9, E14–E16).
//
// # The bug this exists to make impossible
//
// Colour used to be resolved in two places that disagreed: paint() returned a
// block UNPAINTED when its colour resolved to "" (a literal "default", a hex
// value, an unknown key), while joinPowerline() still drew its chevron — an
// unpainted block, a white wedge, a stray triangle. And every filled block
// got white text whatever its background (white on yellow is 1.07:1).
//
// The fix is structural: ONE resolver, ResolveBlockColors, used by both
// painters, that ALWAYS returns a concrete (bg, fg) pair with readable text.
//
// Colours keep their form until emit time: an ANSI index stays an index (0–15
// are recoloured by the terminal's own theme, which is the point of dark8),
// "#rrggbb" stays 24-bit, and Color.SGR downsamples to the terminal's
// ColorMode only when the escape is written.

// RGB is a 24-bit colour.
type RGB struct {
	R, G, B uint8
}

// Color is a resolved colour: an ANSI index (0–255) or 24-bit RGB. The zero
// value is "no colour" (the terminal default).
type Color struct {
	set   bool
	isRGB bool
	index int
	rgb   RGB
}

// Index is the ANSI colour n (0–255).
func Index(n int) Color { return Color{set: true, index: n} }

// FromRGB is a 24-bit colour.
func FromRGB(c RGB) Color { return Color{set: true, isRGB: true, rgb: c} }

// IsZero reports "no colour".
func (c Color) IsZero() bool { return !c.set }

// xterm16 is the conventional RGB of ANSI 0–15. The terminal's theme may
// differ; these are only for contrast and downsampling.
var xterm16 = [16]RGB{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

var cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}

// RGB is the colour's 24-bit value (the xterm default for 0–15).
func (c Color) RGB() RGB {
	switch {
	case c.isRGB:
		return c.rgb
	case c.index < 16:
		return xterm16[c.index]
	case c.index >= 232:
		v := u8(8 + (c.index-232)*10)
		return RGB{v, v, v}
	}
	i := c.index - 16
	return RGB{cubeLevels[i/36], cubeLevels[i/6%6], cubeLevels[i%6]}
}

// themed reports a colour the terminal's theme recolours (ANSI 0–15): its
// real RGB is unknown, so contrast is not computed against it.
func (c Color) themed() bool { return c.set && !c.isRGB && c.index < 16 }

var colorNames = map[string]int{"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7}

// ParseColor reads a theme value: a name ("blue"), an index ("0"–"255"),
// "#rrggbb", or the SGR fragments "38;5;N" / "48;5;N" / "38;2;r;g;b" /
// "48;2;r;g;b". "" and "default" are the zero Color with ok=true; anything
// else — including an ESC byte or an arbitrary fragment — is ok=false.
func ParseColor(v string) (Color, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "default" {
		return Color{}, true
	}
	if strings.ContainsRune(v, '\x1b') {
		return Color{}, false
	}
	if n, ok := colorNames[strings.ToLower(v)]; ok {
		return Index(n), true
	}
	if strings.HasPrefix(v, "#") {
		if len(v) != 7 {
			return Color{}, false
		}
		n, err := strconv.ParseUint(v[1:], 16, 32)
		if err != nil {
			return Color{}, false
		}
		v := int(n) // n < 1<<24: ParseUint above takes six hex digits
		return FromRGB(RGB{u8(v >> 16), u8(v >> 8 & 0xff), u8(v & 0xff)}), true
	}
	parts := strings.Split(v, ";")
	byteOf := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil && n >= 0 && n <= 255
	}
	switch {
	case len(parts) == 1:
		if n, ok := byteOf(v); ok {
			return Index(n), true
		}
	case len(parts) == 3 && (parts[0] == "38" || parts[0] == "48") && parts[1] == "5":
		if n, ok := byteOf(parts[2]); ok {
			return Index(n), true
		}
	case len(parts) == 5 && (parts[0] == "38" || parts[0] == "48") && parts[1] == "2":
		r, okR := byteOf(parts[2])
		g, okG := byteOf(parts[3])
		b, okB := byteOf(parts[4])
		if okR && okG && okB {
			return FromRGB(RGB{u8(r), u8(g), u8(b)}), true
		}
	}
	return Color{}, false
}

// u8 is n clamped to a colour channel.
func u8(n int) uint8 { return uint8(min(max(n, 0), 255)) } // #nosec G115 -- clamped to 0..255

// SGR is the escape that sets c as foreground (bg=false) or background, in
// mode: "" for the zero Color or ModeNever. Indices 0–15 stay indices (themed
// by the terminal) in every mode but 16-colour, where they use the basic
// codes; other colours are downsampled to the nearest the terminal has.
func (c Color) SGR(bg bool, mode ColorMode) string {
	if !c.set || mode == ModeNever {
		return ""
	}
	layer := "38"
	if bg {
		layer = "48"
	}
	switch mode {
	case Mode16:
		n := c.index
		if c.isRGB || n >= 16 {
			n = nearest16(c.RGB())
		}
		base := 30
		if n >= 8 {
			base, n = 90, n-8
		}
		if bg {
			base += 10
		}
		return "\x1b[" + strconv.Itoa(base+n) + "m"
	case ModeTruecolor:
		if c.isRGB {
			return "\x1b[" + layer + ";2;" + strconv.Itoa(int(c.rgb.R)) + ";" + strconv.Itoa(int(c.rgb.G)) + ";" + strconv.Itoa(int(c.rgb.B)) + "m"
		}
	}
	n := c.index
	if c.isRGB {
		n = nearest256(c.rgb)
	}
	return "\x1b[" + layer + ";5;" + strconv.Itoa(n) + "m"
}

func dist(a, b RGB) int {
	dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
	return dr*dr + dg*dg + db*db
}

func nearest16(c RGB) int {
	best := 0
	for i := range xterm16 {
		if dist(c, xterm16[i]) < dist(c, xterm16[best]) {
			best = i
		}
	}
	return best
}

// nearest256 is the closest colour in the 6×6×6 cube or the grey ramp
// (16–255; 0–15 are left alone, as the terminal themes them).
func nearest256(c RGB) int {
	level := func(v uint8) int {
		best := 0
		for i, l := range cubeLevels {
			if absInt(int(v)-int(l)) < absInt(int(v)-int(cubeLevels[best])) {
				best = i
			}
		}
		return best
	}
	cube := 16 + 36*level(c.R) + 6*level(c.G) + level(c.B)
	avg := (int(c.R) + int(c.G) + int(c.B)) / 3
	grey := 232 + max(0, min(23, (avg-8+5)/10))
	if dist(c, Index(grey).RGB()) < dist(c, Index(cube).RGB()) {
		return grey
	}
	return cube
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// luminance is the WCAG relative luminance.
func luminance(c RGB) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// ContrastRatio is the WCAG 2.x contrast ratio between two colours, in
// [1, 21]: used to choose readable text and to gate the builtin palettes.
func ContrastRatio(a, b RGB) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Role names a colour slot: a segment key (its block colour) or a state.
type Role string

const (
	// Segment roles (theme keys).
	RoleDirGit       Role = "dirgit"
	RoleRepoRoot     Role = "repo_root"
	RoleRepoWorktree Role = "repo_worktree"
	RoleAI           Role = "ai"
	RoleTime         Role = "time"
	RoleVideo        Role = "video"
	RoleHost         Role = "host"

	// State roles: the MCP breakdown and the PR badge.
	RoleOK     Role = "ok"     // healthy / connected / open
	RoleWarn   Role = "warn"   // actionable by the user (needs auth)
	RoleErr    Role = "err"    // broken (failed, closed)
	RoleAccent Role = "accent" // emphasis (merged)
	RoleMuted  Role = "muted"  // de-emphasized detail
)

// neutral is the block colour of last resort: a role no palette knows.
var neutral = Index(240)

// ResolveBlockColors returns the concrete (bg, fg) pair for a filled block of
// role.
//
// CONTRACT: it ALWAYS returns two non-zero colours — never "" or a sentinel.
//   - bg: the style's value for role when it parses to a colour, else the
//     dark palette's, else a neutral grey.
//   - fg: the style's "fg" when it parses to a colour (the user always wins),
//     else white on a terminal-themed 0–15 block (its brightness is the
//     terminal's), else whichever of black and white contrasts more with bg
//     (libs/tui/theme.TextOn) — at worst 4.58:1.
func ResolveBlockColors(st Style, role Role) (bg, fg Color) {
	bg = roleColor(st, role)
	if bg.IsZero() {
		bg = neutral
	}
	if c, ok := ParseColor(st.Theme["fg"]); ok && !c.IsZero() {
		return bg, c
	}
	return bg, TextOn(bg)
}

// TextOn is readable default text for a fill of bg. On a terminal-themed
// 0–15 colour: black (0) on the light ones (yellow, white, the bright
// green/yellow/cyan/white), white (7) on the rest — the theme recolours both,
// keeping them apart. Else black or white by contrast.
func TextOn(bg Color) Color {
	if bg.themed() {
		switch bg.index {
		case 3, 7, 10, 11, 14, 15:
			return Index(0)
		}
		return Index(7)
	}
	c := bg.RGB()
	if theme.TextOn("38;2;"+strconv.Itoa(int(c.R))+";"+strconv.Itoa(int(c.G))+";"+strconv.Itoa(int(c.B))) == "16" {
		return Index(16)
	}
	return Index(15)
}

// RoleColor is the colour for role as a tint (no fill) or a chip: the
// style's own value, or the zero Color — plain text — when it has none or it
// does not parse. A style with no theme stays uncoloured.
func RoleColor(st Style, role Role) Color {
	if c, ok := ParseColor(st.Theme[string(role)]); ok {
		return c
	}
	return Color{}
}

// roleColor is a filled block's colour: the style's, else the dark palette's
// (a fill must never be left unpainted), else the zero Color.
func roleColor(st Style, role Role) Color {
	if c := RoleColor(st, role); !c.IsZero() {
		return c
	}
	if c, ok := ParseColor(palettes["dark"][string(role)]); ok {
		return c
	}
	return Color{}
}
