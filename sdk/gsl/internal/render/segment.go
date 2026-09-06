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
//	Segment        — the unit contract. Render(ctx, style, compactLevel) →
//	                  (text, colorKey, ok). text is RAW (unpainted); colorKey
//	                  is the theme key the join layer paints with (e.g. "ai",
//	                  "time", "dirgit", "repo_root", "repo_worktree").
//	                  ok == false means "self-omit" (no data, disabled, error,
//	                  field nil, …); the segment contributes nothing to the line.
//	                  compactLevel 0 = full detail (levels 1–3 are PHASE 2).
//	Render(...)    — the orchestrator. Runs the enabled segments CONCURRENTLY,
//	                  each under its own child-context deadline, then assembles
//	                  segmentBlocks (raw text + colorKey) in config order and
//	                  passes them to the color-aware join layer.
//	glyphs.go      — all glyph lookup + ANSI colouring is contained here.
//	                  join owns all painting: fill blocks AND bridged chevrons.
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
// Render returns the segment's RAW (unpainted) text, the theme colorKey it
// should be painted with (e.g. "ai", "time", "dirgit", "repo_root",
// "repo_worktree"), and an ok flag. When ok is false the segment self-omits:
// the caller drops it and renders the remaining segments. A segment must NEVER
// panic and must honour ctx cancellation/deadline by returning ("", "", false)
// promptly.
//
// compactLevel controls the compaction detail: 0 = full detail. Levels 1–3
// are reserved for PHASE 2 (dynamic width compaction); implementations must
// accept the parameter and treat any non-zero value as level 0 until PHASE 2
// lands.
//
// text must be the raw, unpainted segment content — no embedded ANSI escape
// sequences. The join layer owns all ANSI painting.
type Segment interface {
	Render(ctx context.Context, st style.Style, compactLevel int) (text, colorKey string, ok bool)
}

// LinkedSegment is the OPTIONAL extension a Segment implements when its content
// addresses something on the web — today, the repo segment's PR badge. It is
// the Render-path twin of detect.go's `linkable`; both exist so the two render
// paths stay equivalent (TestDetectFormat_MatchesRender enforces that).
//
// The link travels out as a return value rather than as state stashed on the
// segment because RenderAt renders every segment concurrently: a segment that
// recorded its link in a field would be writing state another goroutine could
// observe mid-flight.
//
// Every span's URL is BARE, never an escape sequence — the join layer owns all
// escape emission, exactly as it owns ANSI painting. No spans means "not
// linkable".
type LinkedSegment interface {
	Segment
	RenderLinked(ctx context.Context, st style.Style, compactLevel int) (text, colorKey string, spans []LinkSpan, ok bool)
}

// LinkSpan is one clickable range of a segment's RAW text: byte offsets
// [Start, End) into the text the segment returned, plus the bare URL. Spans
// carry zero display width; the join layer turns them into OSC 8 hyperlinks.
type LinkSpan struct {
	Start, End int
	URL        string
}
