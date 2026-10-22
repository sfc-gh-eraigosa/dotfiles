package render

import (
	"context"
	"strings"
	"testing"
)

// E17 (plan F11): no segment's format output carries an escape — at any
// level, in either glyph style, root or worktree, linked or not. The join
// layer owns all painting, the PR badge and the MCP breakdown included.
func TestSegments_NoEmbeddedReset(t *testing.T) {
	for _, linked := range []bool{false, true} {
		for _, wt := range []bool{false, true} {
			build := buildGoldenSegments
			if linked {
				build = buildGoldenSegmentsLinked
			}
			segs := build(t, wt, t.TempDir())
			for _, st := range []string{"powerline", "emoji"} {
				style := powerlineStyleFixture()
				if st == "emoji" {
					style = emojiStyleFixture()
				}
				for _, s := range segs {
					d, ok := s.(detectable)
					if !ok {
						continue
					}
					data, ok := d.detect(context.Background())
					if !ok {
						continue
					}
					for level := 0; level <= finalCompactLevel; level++ {
						text, _ := data.format(style, level)
						if strings.ContainsRune(text, 0x1b) {
							t.Errorf("%T level %d (%s, wt=%v, linked=%v) embeds an escape: %q", s, level, st, wt, linked, text)
						}
					}
				}
			}
		}
	}
}
