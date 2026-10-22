package style

import (
	"fmt"
	"strings"
)

// ColorMode is the terminal-capability ladder (plan F10).
type ColorMode int

const (
	ModeAuto      ColorMode = iota // not decided: renderers treat it as Mode256
	ModeNever                      // emit no escapes at all (NO_COLOR, TERM=dumb)
	Mode16                         // basic ANSI
	Mode256                        // ANSI 256-colour
	ModeTruecolor                  // 24-bit
)

func (m ColorMode) String() string {
	return [...]string{"auto", "never", "16", "256", "truecolor"}[m]
}

// ParseColorMode reads a --color value: auto, never, 16, 256 or truecolor.
func ParseColorMode(s string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return ModeAuto, nil
	case "never", "none", "off":
		return ModeNever, nil
	case "16":
		return Mode16, nil
	case "256":
		return Mode256, nil
	case "truecolor", "24bit":
		return ModeTruecolor, nil
	}
	return ModeAuto, fmt.Errorf("--color: %q is not one of auto, never, 16, 256, truecolor", s)
}

// DetectColorMode resolves the mode: an explicit flag wins; else NO_COLOR (any
// value) or TERM=dumb → never; COLORTERM truecolor/24bit → truecolor; a
// terminal known to have 16 colours (linux, vt*, ansi, cons25) → 16; anything
// else → 256. An unparseable flag is ignored (cmd reports it).
func DetectColorMode(env func(string) string, flag string) ColorMode {
	if m, err := ParseColorMode(flag); err == nil && m != ModeAuto {
		return m
	}
	term := strings.ToLower(env("TERM"))
	switch {
	case env("NO_COLOR") != "", term == "dumb":
		return ModeNever
	case strings.Contains(strings.ToLower(env("COLORTERM")), "truecolor"), strings.EqualFold(env("COLORTERM"), "24bit"):
		return ModeTruecolor
	case term == "linux", term == "ansi", term == "cons25", strings.HasPrefix(term, "vt"):
		return Mode16 // known to have only the 16 basic colours
	}
	// A bare TERM (xterm over ssh, screen, tmux — and no TERM at all, as under
	// Claude Code) has meant 256 colours for gsl all along; keep it.
	return Mode256
}
