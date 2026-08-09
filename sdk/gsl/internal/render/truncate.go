package render

// truncate.go — the last resort of the fit ladder, and the reason the width
// invariant (spec E3) is a GUARANTEE rather than a hope.
//
// The shipped Fit had three tiers: escalate text-compaction levels, then drop
// the leading glyphs, then drop whole segments. All three are BEST-EFFORT — each
// makes the line smaller, none of them can make it small ENOUGH. The segment-drop
// loop ran `for len(active) > 1`, so the final surviving segment was emitted
// verbatim no matter how wide it was: a repo named "gsl-ultra-tui-mcp-status"
// produced a 27-column line on a 20-column terminal, which wraps, which eats the
// user's prompt line.
//
// Truncation is what closes that gap. It is the only tier whose output width is
// bounded by construction rather than by luck.
//
// # Grapheme safety
//
// Cutting by BYTES splits a multi-byte rune into mojibake. Cutting by RUNES
// splits a grapheme cluster — "👩‍🚀" is one user-perceived character built from
// three runes joined by a zero-width joiner, and cutting it mid-sequence yields
// two unrelated emoji. Cutting by grapheme cluster but COUNTING clusters gets the
// width wrong: a CJK ideograph is one cluster occupying TWO terminal columns.
//
// So: iterate grapheme clusters (uniseg), accumulate DISPLAY WIDTH, and stop
// before the budget is exceeded. That is the only formulation that is correct for
// all three of ASCII, CJK, and ZWJ emoji simultaneously.

import (
	"strings"

	"github.com/rivo/uniseg"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// ellipsis marks a string that was cut. U+2026 HORIZONTAL ELLIPSIS is a single
// display column — the cheapest possible "there is more here" signal.
const ellipsis = "…"

// truncateText returns the longest grapheme-safe prefix of s whose display width
// is at most cols, with an ellipsis appended when s was actually cut.
//
// Guarantees, for every input:
//   - term.DisplayWidth(result) <= cols
//   - result is valid UTF-8 and never splits a grapheme cluster
//   - result is non-empty whenever cols >= 1 and s is non-empty
//
// s must be RAW text (no ANSI escapes) — see truncateToWidth, which strips first.
func truncateText(s string, cols int) string {
	if cols <= 0 || s == "" {
		return ""
	}
	if term.DisplayWidth(s) <= cols {
		return s
	}
	// One column of room: all we can say is "there was something here".
	if cols == 1 {
		return ellipsis
	}

	budget := cols - 1 // reserve one column for the ellipsis
	var b strings.Builder
	width := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		cw := uniseg.StringWidth(cluster)
		if width+cw > budget {
			break
		}
		b.WriteString(cluster)
		width += cw
	}
	return b.String() + ellipsis
}

// joinOverhead returns the number of display columns that join() spends on
// DECORATION for the given block count — padding, separators, and powerline
// chevrons — i.e. everything that is not segment text.
//
// It is measured, not assumed, because the two join paths (powerline-with-fill
// vs the classic separator path) have different per-block costs and a style can
// disable the chevron glyph entirely. Measuring the real join keeps this correct
// even if the join layer changes.
//
// The overhead depends only on the NUMBER of blocks, not on their text, so it is
// still valid after the text has been truncated.
func joinOverhead(st style.Style, blocks []segmentBlock) int {
	if len(blocks) == 0 {
		return 0
	}
	total := term.DisplayWidth(join(st, blocks))
	for _, b := range blocks {
		total -= term.DisplayWidth(b.text)
	}
	if total < 0 {
		return 0
	}
	return total
}

// truncateToWidth shortens blocks so that join(st, result) fits within cols.
//
// It is the frozen §3 contract: pure, grapheme-safe, and it never touches I/O.
//
// Returns nil when the decoration alone (padding + separators + chevrons) is
// already wider than cols — i.e. when NO decorated line can fit, however empty.
// A nil return is the caller's signal to fall back to an undecorated line; it is
// not an error.
func truncateToWidth(blocks []segmentBlock, st style.Style, cols int) []segmentBlock {
	if len(blocks) == 0 || cols <= 0 {
		return nil
	}
	// Fast path: it already fits, so leave the styling entirely alone.
	if term.DisplayWidth(join(st, blocks)) <= cols {
		return blocks
	}

	// Strip ANSI before cutting. A block's text is supposed to be raw (the
	// segment.go:12 invariant), but prBadge currently embeds an SGR tint — and a
	// cut that lands inside "\x1b[38;5;2m" would spray "38;5;2m" onto the line.
	// The join layer re-applies the block's colour anyway, so nothing visible is
	// lost by dropping the sub-field tint in this last-resort path.
	work := make([]segmentBlock, len(blocks))
	for i, b := range blocks {
		work[i] = segmentBlock{text: term.StripANSI(b.text), colorKey: b.colorKey}
	}

	// Shed trailing blocks until the decoration leaves at least one column for
	// text. (Fit normally hands us a single survivor, but truncateToWidth must be
	// correct for any block count.)
	for len(work) > 1 && joinOverhead(st, work) >= cols {
		work = work[:len(work)-1]
	}
	budget := cols - joinOverhead(st, work)
	if budget <= 0 {
		return nil
	}

	// Spend the budget left to right. A block that cannot get at least one column
	// is dropped; the block that runs out of budget mid-way is ellipsized.
	//
	// Dropping blocks here only REDUCES the decoration overhead below what
	// `budget` was computed against, so the result is never wider than cols.
	out := make([]segmentBlock, 0, len(work))
	remaining := budget
	for _, b := range work {
		w := term.DisplayWidth(b.text)
		if w <= remaining {
			out = append(out, b)
			remaining -= w
			continue
		}
		if remaining >= 1 {
			out = append(out, segmentBlock{
				text:     truncateText(b.text, remaining),
				colorKey: b.colorKey,
			})
		}
		break
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
