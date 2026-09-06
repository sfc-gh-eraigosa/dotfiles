package cmd

// columns_test.go — spec gsl-ultra E1, E2 (feature F1: width resolution).
//
// The four HOST scenarios, as a table. Every input is injected — the env lookup
// and both tty probes are function parameters — so each branch is exercised
// deterministically without a pty and without touching the process environment.

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// tty returns a width source that reports a live terminal of n columns.
func tty(n int) func() (int, bool) { return func() (int, bool) { return n, true } }

// piped returns a width source that reports "not a terminal" — what the ioctl
// wrapper does when the fd is a pipe, which is exactly how Claude Code invokes
// the status-line command.
func piped() func() (int, bool) { return func() (int, bool) { return 0, false } }

// envOf builds an env lookup over a fixed map.
func envOf(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func intptr(n int) *int { return &n }

func TestResolveColumns(t *testing.T) {
	tests := []struct {
		name       string
		p          payload.Payload
		env        map[string]string
		ttyStdout  func() (int, bool)
		ttyStderr  func() (int, bool)
		wantCols   int
		wantSource string
	}{
		{
			// E1 — the host told us the width. Most authoritative source.
			name:       "a/payload width wins over everything, stdout piped",
			p:          payload.Payload{TerminalWidth: intptr(200)},
			env:        map[string]string{"COLUMNS": "90"},
			ttyStdout:  piped(),
			ttyStderr:  tty(80),
			wantCols:   200,
			wantSource: sourcePayload,
		},
		{
			// E2 — THE KEY CASE. This is how Claude Code actually runs gsl:
			// stdout is a pipe, no terminal_width in the payload, but stderr is
			// still attached to the real tty. Nothing in the shipped code looks
			// at stderr, so every real render fell through to the hardcoded 80.
			name:       "b/stdout piped, no payload width, stderr is a tty (Claude)",
			p:          payload.Payload{},
			env:        map[string]string{},
			ttyStdout:  piped(),
			ttyStderr:  tty(80),
			wantCols:   80,
			wantSource: sourceStderr,
		},
		{
			// c — plain interactive shell usage: stdout IS the terminal.
			name:       "c/stdout is a tty",
			p:          payload.Payload{},
			env:        map[string]string{},
			ttyStdout:  tty(100),
			ttyStderr:  tty(80),
			wantCols:   100,
			wantSource: sourceStdout,
		},
		{
			// d — neither fd is a tty, but COLUMNS was explicitly exported.
			name:       "d/no tty anywhere, COLUMNS exported",
			p:          payload.Payload{},
			env:        map[string]string{"COLUMNS": "90"},
			ttyStdout:  piped(),
			ttyStderr:  piped(),
			wantCols:   90,
			wantSource: sourceColumns,
		},
		{
			// e — nothing knows. 120, not the teletype-era 80.
			name:       "e/nothing at all",
			p:          payload.Payload{},
			env:        map[string]string{},
			ttyStdout:  piped(),
			ttyStderr:  piped(),
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},

		// ── edges ────────────────────────────────────────────────────────────
		{
			name:       "edge/payload width 0 falls through",
			p:          payload.Payload{TerminalWidth: intptr(0)},
			env:        map[string]string{},
			ttyStdout:  piped(),
			ttyStderr:  tty(80),
			wantCols:   80,
			wantSource: sourceStderr,
		},
		{
			name:       "edge/payload width negative falls through",
			p:          payload.Payload{TerminalWidth: intptr(-5)},
			env:        map[string]string{},
			ttyStdout:  piped(),
			ttyStderr:  piped(),
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},
		{
			name:       "edge/COLUMNS unparseable falls through to default",
			p:          payload.Payload{},
			env:        map[string]string{"COLUMNS": "not-a-number"},
			ttyStdout:  piped(),
			ttyStderr:  piped(),
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},
		{
			name:       "edge/COLUMNS zero falls through to default",
			p:          payload.Payload{},
			env:        map[string]string{"COLUMNS": "0"},
			ttyStdout:  piped(),
			ttyStderr:  piped(),
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},
		{
			name:       "edge/nil probes are safe",
			p:          payload.Payload{},
			env:        map[string]string{},
			ttyStdout:  nil,
			ttyStderr:  nil,
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},
		{
			name:       "edge/tty reports ok but zero width falls through",
			p:          payload.Payload{},
			env:        map[string]string{},
			ttyStdout:  func() (int, bool) { return 0, true },
			ttyStderr:  func() (int, bool) { return 0, true },
			wantCols:   fallbackColumns,
			wantSource: sourceDefault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCols, gotSource := resolveColumns(tc.p, envOf(tc.env), tc.ttyStdout, tc.ttyStderr)
			if gotCols != tc.wantCols || gotSource != tc.wantSource {
				t.Errorf("resolveColumns() = (%d, %q), want (%d, %q)",
					gotCols, gotSource, tc.wantCols, tc.wantSource)
			}
		})
	}
}

// TestResolveColumns_NilEnv proves a nil env lookup does not panic (the status
// line must never crash — UC-7).
func TestResolveColumns_NilEnv(t *testing.T) {
	cols, src := resolveColumns(payload.Payload{}, nil, piped(), piped())
	if cols != fallbackColumns || src != sourceDefault {
		t.Errorf("resolveColumns(nil env) = (%d, %q), want (%d, %q)",
			cols, src, fallbackColumns, sourceDefault)
	}
}
