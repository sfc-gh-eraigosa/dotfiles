package cmd

import (
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// columns.go — the FROZEN width-resolution contract (gsl-ultra, plan §3).
//
// # The bug this exists to fix
//
// The shipped resolution order is:
//
//	$COLUMNS  →  ioctl(stdout)  →  80
//
// Both of the first two branches are DEAD in production:
//
//   - $COLUMNS is a SHELL variable. It is not exported to child processes.
//     Verified: `env | grep -c '^COLUMNS='` → 0. gsl never sees it.
//   - Claude Code invokes the status-line command with stdout PIPED, so the
//     ioctl on stdout fails.
//
// Claude sends no terminal_width, and `agy status` sends no payload at all.
// So EVERY real render falls through to the hardcoded 80 — on a 200-column
// terminal, gsl compacts a line that had 120 columns of room to spare.
//
// # The fix
//
// A single pure, injectable resolver with an explicit precedence, which also
// reports WHICH source won so the choice is testable and loggable.
//
// The load-bearing insight is the STDERR probe: Claude pipes stdout but
// routinely leaves stderr attached to the tty. When the payload omits a width,
// ioctl(stderr) is a free, accurate source of the REAL terminal width. Nothing
// in the shipped code looks there.

// fallbackColumns is the last resort when no source knows the width.
//
// 120, not 80. 80 is a teletype default that under-serves every modern
// terminal; when we are guessing, guess generously — an over-wide guess is
// corrected by Fit's truncation (which is now a hard guarantee), whereas an
// under-wide guess silently discards information the user had room for.
const fallbackColumns = 120

// widthSource names the origin of the resolved column count. Returned alongside
// the width so tests can assert WHICH branch fired (not merely that the number
// looks plausible) and so the choice can be logged at Debug.
const (
	sourcePayload = "payload"      // payload.terminal_width (the host told us)
	sourceStdout  = "ioctl:stdout" // stdout is a tty
	sourceStderr  = "ioctl:stderr" // stdout piped, stderr still on the tty
	sourceColumns = "env:COLUMNS"  // exported COLUMNS (rare; honoured if present)
	sourceDefault = "default"      // nothing knew; fallbackColumns
)

// resolveColumns determines the terminal width and the source that supplied it.
//
// PURE and fully injected: env and the two tty probes are parameters, so all
// four host scenarios (Claude piped w/ payload, Claude piped w/o payload, agy,
// a plain shell tty) are a table test rather than a guess.
//
// Precedence:
//
//  1. payload.terminal_width — the host explicitly told us. Most authoritative.
//  2. ioctl(stdout)          — stdout is a tty (plain shell usage).
//  3. ioctl(stderr)          — stdout is piped but stderr is a tty (CLAUDE).
//  4. $COLUMNS               — only if actually exported. Usually absent.
//  5. fallbackColumns        — 120.
//
// Note the inversion versus the shipped code: $COLUMNS now ranks BELOW the
// host-supplied width. Previously an exported COLUMNS would override the
// terminal width the host itself reported, which is backwards.
//
// ttyStdout and ttyStderr each return (cols, ok); ok=false means "not a tty" or
// "the ioctl failed". Either may be nil (treated as always-false).
func resolveColumns(
	p payload.Payload,
	env func(string) string,
	ttyStdout func() (int, bool),
	ttyStderr func() (int, bool),
) (cols int, source string) {
	// 1. The host explicitly told us. Nothing beats that.
	if p.TerminalWidth != nil && *p.TerminalWidth > 0 {
		return *p.TerminalWidth, sourcePayload
	}

	// 2. stdout is a terminal (plain interactive shell usage).
	if n, ok := probe(ttyStdout); ok {
		return n, sourceStdout
	}

	// 3. stdout is a pipe but stderr is still on the tty. THIS is the Claude
	//    Code case, and the branch that did not exist before.
	if n, ok := probe(ttyStderr); ok {
		return n, sourceStderr
	}

	// 4. An explicitly EXPORTED COLUMNS. Usually absent (it is a shell variable,
	//    not an environment variable), but honour it when a user exports it.
	if env != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(env("COLUMNS"))); err == nil && n > 0 {
			return n, sourceColumns
		}
	}

	// 5. Nothing knew.
	return fallbackColumns, sourceDefault
}

// probe calls a tty width source defensively: a nil source, a source reporting
// "not a tty", and a source reporting a nonsensical width are all "unknown".
func probe(src func() (int, bool)) (int, bool) {
	if src == nil {
		return 0, false
	}
	n, ok := src()
	if !ok || n <= 0 {
		return 0, false
	}
	return n, true
}
