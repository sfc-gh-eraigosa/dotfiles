// Package render is the integration layer that turns the detection packages
// (git, repo, mcp), the Claude payload, the config, and the resolved style
// into the final rendered status line.
//
// render is NOT a seam: it never imports os/exec. All subprocess work is
// delegated to the git.Runner / gh.Runner / mcp.Runner interfaces that the
// detection packages already wrap. render only composes their results.
//
// # Architecture
//
//	Segment        — the unit contract. Render(ctx, style) → (text, ok).
//	                  ok == false means "self-omit" (no data, disabled, error,
//	                  field nil, …); the segment contributes nothing to the line.
//	Render(...)    — the orchestrator. Runs the enabled segments CONCURRENTLY,
//	                  each under its own child-context deadline, then joins the
//	                  surviving texts in config order using the style's
//	                  separator / fill / theme.
//	glyphs.go      — all glyph lookup + ANSI colouring is contained here.
//	seg_*.go       — the four concrete segments (dirgit, repo, ai, time).
//
// Every dependency a segment needs (runners, the parsed payload, a clock, the
// config options) is injected at construction time so tests can drive the
// segments with fakes and a fixed clock for deterministic golden output.
package render

import (
	"context"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// Segment is the contract every status-line segment implements.
//
// Render returns the segment's rendered text (already glyph- and colour-
// applied for the given style) and an ok flag. When ok is false the segment
// self-omits: the caller drops it and renders the remaining segments. A
// segment must NEVER panic and must honour ctx cancellation/deadline by
// returning ("", false) promptly.
type Segment interface {
	Render(ctx context.Context, st style.Style) (text string, ok bool)
}
