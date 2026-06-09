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
package term

import (
	"os"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"
)

// Columns resolves the current terminal column count.
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

// ansiStripper removes ANSI SGR escape sequences (ESC [ ... m) from s.
// It handles only the SGR form this codebase emits; other OSC / DEC sequences
// are left intact (they don't appear in gsl output).
func ansiStripper(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
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
