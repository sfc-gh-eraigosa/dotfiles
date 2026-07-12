package render

// detect.go — the Detect-once / Format-per-level infrastructure.
//
// Key invariant: ALL subprocess I/O (git, gh, mcp) happens inside Detect.
// Format and Fit are PURE: no I/O, no goroutines, just transformations on
// the cached segmentData slice.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
	"github.com/sirupsen/logrus"
)

// finalCompactLevel is the highest level the Fit loop will try.
// Levels 0..finalCompactLevel-1 are text compaction; finalCompactLevel is the
// final tier (glyph-drop then segment-drop).
const finalCompactLevel = 4

// detectable is the interface that segments implement to participate in
// the detect-once / format-per-level flow.
//
// detect runs ALL I/O and returns the segment's raw detected data. ok == false
// means the segment self-omits (no data, disabled, error, …).
type detectable interface {
	detect(ctx context.Context) (segmentData, bool)
}

// segmentData is the level-independent, I/O-free intermediate produced by
// detect. Each concrete segment type implements its own segmentData and
// the format method.
type segmentData interface {
	// format returns raw (unpainted) text and the theme colorKey at the given
	// compaction level. PURE: no I/O. Returns ("", "") to self-omit at this level.
	//
	// format MUST be monotonically non-increasing in width as level rises
	// (spec E5): a segment that ignores its level silently defeats the entire
	// compaction ladder and forces Fit into the segment-drop tier far earlier
	// than necessary.
	format(st style.Style, level int) (text, colorKey string)
}

// prioritized is the optional interface a segmentData implements to declare its
// DROP priority — which segment Fit sacrifices first when the line still does
// not fit after text compaction.
//
// Priority is deliberately NOT slice position. Before this existed, Fit dropped
// the right-most element of cfg.Segments, so a user who merely REORDERED their
// status line (a cosmetic preference) silently changed which information they
// could still read on a narrow terminal. Position is presentation; priority is
// policy. They are different questions and now have different answers.
type prioritized interface {
	priority() int
}

// priorityOf returns d's drop priority. A segmentData that declares none sorts
// with the first-dropped group.
func priorityOf(d segmentData) int {
	if p, ok := d.(prioritized); ok {
		return p.priority()
	}
	return 0
}

// Detect runs the enabled segments CONCURRENTLY (one goroutine each, with a
// per-segment deadline), collecting each segment's detected data without any
// formatting or painting. This is where ALL subprocess I/O happens.
//
// The returned slice is ordered to match segs: a segment that self-omits or
// times out contributes a nil entry that Format skips.
func Detect(ctx context.Context, cfg config.Config, st style.Style, segs []Segment) []segmentData {
	if !cfg.Enabled || len(segs) == 0 {
		return nil
	}

	type item struct {
		data segmentData
		ok   bool
	}
	items := make([]item, len(segs))

	var wg sync.WaitGroup
	wg.Add(len(segs))

	for i, seg := range segs {
		go func(idx int, s Segment) {
			defer wg.Done()
			// Recover so a panicking segment is dropped, not fatal.
			defer func() {
				if r := recover(); r != nil {
					observe.Default().WithFields(logrus.Fields{
						"event":   "segment.panic",
						"segment": segmentTypeName(s),
						"panic":   fmt.Sprintf("%v", r),
					}).Warn("segment panicked during detect; dropping")
					items[idx] = item{ok: false}
				}
			}()

			sctx, cancel := context.WithTimeout(ctx, segmentDeadline)
			defer cancel()

			// Check if the segment supports the new detect interface.
			if d, ok := s.(detectable); ok {
				data, ok2 := d.detect(sctx)
				if sctx.Err() == context.DeadlineExceeded {
					observe.Default().WithFields(logrus.Fields{
						"event":       "segment.timeout",
						"segment":     segmentTypeName(s),
						"deadline_ms": segmentDeadline.Milliseconds(),
					}).Warn("segment exceeded per-segment deadline during detect; dropping")
					items[idx] = item{ok: false}
					return
				}
				items[idx] = item{data: data, ok: ok2}
				return
			}

			// Fallback: segment doesn't implement detectable. Run Render at level 0
			// to get the data, wrap it in a staticSegmentData so Format can still use it.
			text, colorKey, ok2 := s.Render(sctx, st, 0)
			if sctx.Err() == context.DeadlineExceeded {
				observe.Default().WithFields(logrus.Fields{
					"event":       "segment.timeout",
					"segment":     segmentTypeName(s),
					"deadline_ms": segmentDeadline.Milliseconds(),
				}).Warn("segment exceeded per-segment deadline during detect fallback; dropping")
				items[idx] = item{ok: false}
				return
			}
			if !ok2 || text == "" {
				items[idx] = item{ok: false}
				return
			}
			items[idx] = item{
				data: &staticSegmentData{text: text, colorKey: colorKey},
				ok:   true,
			}
		}(i, seg)
	}
	wg.Wait()

	out := make([]segmentData, len(segs))
	for idx, it := range items {
		if it.ok {
			out[idx] = it.data
		}
	}
	return out
}

// staticSegmentData is a no-op segmentData for segments that do not implement
// the detectable interface. It returns the same text+colorKey at every level.
type staticSegmentData struct {
	text     string
	colorKey string
}

func (s *staticSegmentData) format(_ style.Style, _ int) (string, string) {
	return s.text, s.colorKey
}

// Format is PURE: it formats each segmentData at compactLevel, assembles the
// surviving blocks, and passes them to the color-aware join layer. No I/O.
//
// level 0 = full detail; levels 1..finalCompactLevel-1 = per-segment text
// compaction; finalCompactLevel = final tier (glyph-drop → segment-drop).
func Format(datas []segmentData, st style.Style, level int) string {
	if len(datas) == 0 {
		return ""
	}

	// Final tier: drop leading glyphs, then drop lowest-priority segments.
	if level >= finalCompactLevel {
		return formatFinalTier(datas, st)
	}

	blocks := make([]segmentBlock, 0, len(datas))
	for _, d := range datas {
		if d == nil {
			continue
		}
		text, colorKey := d.format(st, level)
		if text != "" {
			blocks = append(blocks, segmentBlock{text: text, colorKey: colorKey})
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return join(st, blocks)
}

// formatFinalTier applies the final compaction tier:
//  1. Re-format at the deepest text level (finalCompactLevel-1).
//  2. For each surviving block, drop the per-segment leading glyph.
//  3. Return the glyph-dropped output (the Fit loop will call this when
//     narrower widths still don't fit, but that's the caller's concern —
//     this tier is the most compact single Format call).
//
// Both powerline and emoji use this tier; emoji is the binding case because
// each emoji glyph is an irreducible ~2 cols + space.
func formatFinalTier(datas []segmentData, st style.Style) string {
	blocks := finalTierBlocks(datas, st)
	if len(blocks) == 0 {
		return ""
	}
	return join(st, blocks)
}

// finalTierBlocks is formatFinalTier's block list, before the join. Fit's
// truncation tier needs the blocks themselves (it has to shorten their text),
// not the already-assembled string.
func finalTierBlocks(datas []segmentData, st style.Style) []segmentBlock {
	textLevel := finalCompactLevel - 1

	blocks := make([]segmentBlock, 0, len(datas))
	for _, d := range datas {
		if d == nil {
			continue
		}
		text, colorKey := d.format(st, textLevel)
		if text == "" {
			continue
		}
		// Drop the leading glyph.
		text = dropLeadingGlyph(text)
		if text == "" {
			continue
		}
		blocks = append(blocks, segmentBlock{text: text, colorKey: colorKey})
	}
	return blocks
}

// dropLeadingGlyph removes the leading non-ASCII glyph (emoji or Nerd Font
// icon) and the space that follows it from text. If the first rune is ASCII
// or there is no trailing space, text is returned unchanged.
//
// Examples:
//
//	"📁 mydir branch" → "mydir branch"
//	"🤖 Opus model"  → "Opus model"
//	"project"        → "project"  (ASCII; unchanged)
func dropLeadingGlyph(text string) string {
	if text == "" {
		return text
	}
	r := []rune(text)
	if len(r) == 0 {
		return text
	}
	first := r[0]
	// ASCII printable → not a glyph.
	if first < 128 {
		return text
	}
	// Multi-byte glyph. Check if followed by space(s).
	rest := string(r[1:])
	trimmed := strings.TrimLeft(rest, " ")
	if len(trimmed) < len(rest) {
		// There was at least one space — glyph+space stripped.
		return trimmed
	}
	// No space after the glyph — return as-is.
	return text
}

// Fit renders datas into a line that is GUARANTEED to be at most cols display
// columns wide (spec E3), and non-empty whenever any segment has content (E4).
//
// The ladder, cheapest concession first:
//
//	Phase 1  escalate text-compaction levels 0..finalCompactLevel.
//	Phase 2  drop whole segments, LOWEST PRIORITY FIRST (E6) — not right-most.
//	Phase 3  truncate the survivor, grapheme-safely, with an ellipsis (E3).
//
// Phases 1 and 2 are best-effort: they make the line smaller, but neither can
// make it small ENOUGH. Only phase 3 is bounded by construction. The shipped
// code stopped after phase 2 with `for len(active) > 1`, which left the final
// surviving segment un-truncated — a 27-column line on a 20-column terminal.
//
// Fit is PURE (no I/O) — all data is already in datas from a prior Detect call.
func Fit(datas []segmentData, st style.Style, cols int) string {
	if len(datas) == 0 || cols <= 0 {
		return ""
	}

	// ── Phase 1: escalate through text levels 0..finalCompactLevel. ──────────
	for level := 0; level <= finalCompactLevel; level++ {
		out := Format(datas, st, level)
		if out == "" {
			// Nothing to render at all (every segment self-omitted).
			return ""
		}
		if term.DisplayWidth(out) <= cols {
			return out
		}
	}

	// ── Phase 2: drop segments, lowest priority first. ───────────────────────
	//
	// The VISUAL order of the survivors is unchanged (pruned keeps each segment
	// at its original index); only the DROP order is by priority.
	pruned := make([]segmentData, len(datas))
	copy(pruned, datas)

	for _, idx := range dropOrder(datas) {
		if countActive(pruned) <= 1 {
			break
		}
		pruned[idx] = nil

		out := formatFinalTier(pruned, st)
		if out == "" {
			break
		}
		if term.DisplayWidth(out) <= cols {
			return out
		}
	}

	// ── Phase 3: truncate the survivor. ──────────────────────────────────────
	blocks := finalTierBlocks(pruned, st)
	if len(blocks) == 0 {
		return ""
	}

	if fitted := truncateToWidth(blocks, st, cols); len(fitted) > 0 {
		if out := join(st, fitted); term.DisplayWidth(out) <= cols {
			return out
		}
	}

	// The decoration itself does not fit (e.g. cols=2 with powerline, whose
	// padding + chevron alone costs 3 columns). Emit an UNDECORATED, painted,
	// grapheme-safe truncation: no padding, no separator, no chevron — just as
	// much of the highest-priority segment as the terminal can physically hold.
	text := truncateText(term.StripANSI(blocks[0].text), cols)
	if text == "" {
		return ""
	}
	return paint(st, blocks[0].colorKey, text)
}

// dropOrder returns the indices of the non-nil segments in the order Fit should
// sacrifice them: ASCENDING priority, and — among equal priorities — right-most
// first, which preserves the historical behaviour for segments that declare no
// priority of their own.
func dropOrder(datas []segmentData) []int {
	order := make([]int, 0, len(datas))
	for i, d := range datas {
		if d != nil {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		pa, pb := priorityOf(datas[order[a]]), priorityOf(datas[order[b]])
		if pa != pb {
			return pa < pb
		}
		return order[a] > order[b]
	})
	return order
}

// countActive returns the number of non-nil entries in datas.
func countActive(datas []segmentData) int {
	n := 0
	for _, d := range datas {
		if d != nil {
			n++
		}
	}
	return n
}
