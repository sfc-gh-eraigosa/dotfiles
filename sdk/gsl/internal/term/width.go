// Package term provides terminal-width helpers for the gsl status line.
//
// Two primary functions:
//
//	Columns(source) — resolves the terminal column count: $COLUMNS env var
//	                  wins first, then the injected source (production: ioctl
//	                  TIOCGWINSZ on stdout), then the hard fallback 80. The
//	                  injected source keeps the ioctl path testable without a
//	                  real tty.
//
//	DisplayWidth(s) — strips ANSI SGR escape sequences then measures the
//	                  resulting string with github.com/rivo/uniseg
//	                  (grapheme-cluster-aware, East-Asian-wide-aware).
//	                  The value is the uniseg.StringWidth result — deterministic
//	                  and font/terminal-independent.
//
//	StdoutWidthSource() — returns the production width source: wraps
//	                      charmbracelet/x/term.GetSize(os.Stdout.Fd()). Returns
//	                      (0, false) when stdout is not a TTY (piped, as under
//	                      Claude Code), allowing Columns to fall back to $COLUMNS
//	                      then 80.
package term

import (
	"os"
	"strconv"
	"strings"

	cxterm "github.com/charmbracelet/x/term"
	"github.com/rivo/uniseg"
)

// Columns resolves the current terminal column count.
//
// Deprecated: Columns implements the SUPERSEDED resolution order and must not be
// used on any new path. Use cmd.resolveColumns (spec F1) instead, which resolves
// payload.terminal_width → ioctl(stdout) → ioctl(stderr) → $COLUMNS →
// cfg.FallbackColumns and reports which source won.
//
// Two defects are preserved here only because internal/preview still calls this
// for its pre-WindowSizeMsg first frame:
//
//   - $COLUMNS is ranked ABOVE the ioctl probes. It is a shell variable that is
//     not exported to child processes, so in practice it is absent — but when a
//     user DOES export it, it wrongly overrides the real terminal width.
//   - There is no ioctl(stderr) probe, and the hard fallback is 80. Claude Code
//     pipes stdout, so this function returns 80 on a 200-column terminal. That is
//     precisely the bug the width leaf exists to fix; do not reintroduce it by
//     reaching for this helper.
//
// Resolution order:
//  1. $COLUMNS environment variable (if set and parses as a positive integer).
//  2. The injected source (production: wraps ioctl TIOCGWINSZ on stdout).
//  3. Hard fallback: 80.
//
// source may be nil; it is treated as always returning (0, false).
func Columns(source func() (int, bool)) int {
	// 1. $COLUMNS wins over everything.
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	// 2. Injected source (e.g. ioctl wrapper).
	if source != nil {
		if n, ok := source(); ok && n > 0 {
			return n
		}
	}
	// 3. Fallback.
	return 80
}

// StdoutWidthSource returns the production ioctl width source. It queries
// the terminal size of os.Stdout via TIOCGWINSZ (on Unix) using the
// charmbracelet/x/term package (already in the module graph via bubbletea).
//
// Returns (0, false) when:
//   - stdout is not a TTY (piped output, as under Claude Code's status-line
//     command), so Columns falls back to $COLUMNS then 80.
//   - GetSize returns an error (unsupported platform, closed fd, etc.).
//
// The source signature matches the func() (int, bool) expected by Columns.
func StdoutWidthSource() func() (int, bool) {
	return func() (int, bool) {
		w, _, err := cxterm.GetSize(os.Stdout.Fd())
		if err != nil || w <= 0 {
			return 0, false
		}
		return w, true
	}
}

// StderrWidthSource returns an ioctl width source for os.Stderr.
//
// This is the source that makes gsl work under Claude Code. Claude invokes the
// status-line command with stdout attached to a PIPE (it captures the line), so
// the stdout ioctl always fails — but it leaves stderr attached to the real
// terminal. Probing stderr is therefore a free, accurate read of the user's
// actual terminal width in exactly the case where every other source is blind.
//
// Returns (0, false) when stderr is not a TTY or GetSize fails.
func StderrWidthSource() func() (int, bool) {
	return func() (int, bool) {
		w, _, err := cxterm.GetSize(os.Stderr.Fd())
		if err != nil || w <= 0 {
			return 0, false
		}
		return w, true
	}
}

// StripANSI removes ANSI SGR escape sequences from s, returning the raw text.
//
// Truncation must operate on raw text: cutting a string mid-escape-sequence
// emits the sequence's tail as literal garbage. Callers that need to shorten a
// possibly-painted string strip first, cut second, and let the paint layer
// re-apply the colour.
func StripANSI(s string) string { return ansiStripper(s) }

// ansiStripper removes ANSI escape sequences from s: CSI (ESC [ ... final) —
// which covers the SGR colour sequences this codebase emits — and OSC
// (ESC ] ... terminator), which covers the OSC 8 hyperlinks the join layer
// wraps around linkable segments.
//
// OSC 8 matters for width: the sequence carries a full URL, and every one of
// those bytes would otherwise be counted as display width by DisplayWidth. A
// ~55-column PR URL would make the fit loop believe the line is 55 columns
// wider than it is and start shedding segments that fit perfectly well.
//
// Both ST (ESC \) and the legacy BEL terminator are accepted, because
// terminals differ on which they emit and gsl must measure either correctly.
// An unterminated OSC consumes to end-of-string: that is the safe direction —
// a partial escape is not display width either.
func ansiStripper(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == ']' {
			// OSC: ESC ] <params> <ST | BEL>, where ST is ESC \.
			j := i + 2
			for j < len(s) {
				if s[j] == '\a' { // BEL
					j++
					break
				}
				if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' { // ST
					j += 2
					break
				}
				j++
			}
			i = j
			continue
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Scan past the CSI sequence: ESC [ <params> <final>
			// Final byte is in range 0x40–0x7E.
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7E) {
				j++
			}
			if j < len(s) {
				j++ // consume the final byte
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// DisplayWidth returns the number of terminal columns that s occupies, after
// stripping ANSI SGR sequences. It uses github.com/rivo/uniseg for grapheme-
// cluster-aware, East-Asian-wide-aware counting.
//
// The returned value is the uniseg.StringWidth result — deterministic across
// platforms regardless of font or terminal configuration. Tests assert this
// value; they do NOT assert physical cells in any particular terminal.
func DisplayWidth(s string) int {
	return uniseg.StringWidth(ansiStripper(s))
}
