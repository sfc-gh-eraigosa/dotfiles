package render

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// aiParts are the four segments the old ai segment became (#426).
func aiParts(t *testing.T) []Segment {
	t.Helper()
	var out []Segment
	for _, p := range []string{"model", "context", "mcp", "limits"} {
		s := NewAISegment(samplePayload(), t.TempDir(), mcpOneOfTwo())
		s.Part = p
		out = append(out, s)
	}
	return out
}

func detectAll(t *testing.T, segs []Segment) []segmentData {
	t.Helper()
	var out []segmentData
	for _, s := range segs {
		d, ok := s.(detectable).detect(context.Background())
		if !ok {
			t.Fatalf("%T did not detect", s)
		}
		out = append(out, d)
	}
	return out
}

// #426 done-when: the four parts side by side draw exactly the old ai
// segment — at every compaction level before the final tier, and through Fit
// at any width that does not reach it — in both glyph styles.
func TestAISplit_ByteIdenticalToAI(t *testing.T) {
	legacy := NewAISegment(samplePayload(), t.TempDir(), mcpOneOfTwo())
	for _, st := range []style.Style{powerlineStyleFixture(), emojiStyleFixture()} {
		old := detectAll(t, []Segment{legacy})
		split := detectAll(t, aiParts(t))
		for level := 0; level < finalCompactLevel; level++ {
			if a, b := Format(old, st, level), Format(split, st, level); a != b {
				t.Errorf("%s level %d:\n old %q\nsplit %q", st.Separator, level, a, b)
			}
		}
		for _, cols := range []int{200, 80, 50, 40} {
			if a, b := Fit(old, st, cols), Fit(split, st, cols); a != b {
				t.Errorf("%s Fit(%d):\n old %q\nsplit %q", st.Separator, cols, a, b)
			}
		}
	}
}

// Each part alone is just its piece, at its own ladder.
func TestAISplit_PartsAlone(t *testing.T) {
	st := emojiStyleFixture()
	parts := detectAll(t, aiParts(t))
	cases := []struct {
		i     int
		level int
		want  string
		not   string
	}{
		{0, 0, "Opus 4.7", "42%"},
		{1, 0, "42% 84k/200k", "Opus"},
		{1, 2, "42%", "84k"},
		{2, 0, "1✓ 1✗", "42%"},
		{2, 3, "1", "✓"},
		{3, 0, "5h 80% 7d 15%", "Opus"},
		{3, 1, "5h 80%", "7d"},
	}
	for _, c := range cases {
		text, key := parts[c.i].format(st, c.level)
		if !strings.Contains(text, c.want) || (c.not != "" && strings.Contains(text, c.not)) || key != "ai" {
			t.Errorf("part %d level %d = %q (%s), want %q without %q", c.i, c.level, text, key, c.want, c.not)
		}
	}
	if text, _ := parts[3].format(st, 2); text != "" {
		t.Errorf("limits at level 2 should be empty (dropped), got %q", text)
	}
}

// Parts that are NOT side by side are separate blocks: mcp moved next to the
// repo keeps its own place.
func TestAISplit_Reordered(t *testing.T) {
	st := emojiStyleFixture()
	parts := detectAll(t, aiParts(t))
	line := term.StripANSI(Format([]segmentData{parts[2], &staticSegmentData{text: "REPO", colorKey: "repo_root"}, parts[0]}, st, 0))
	if !strings.Contains(line, "1✓ 1✗") || strings.Index(line, "1✓") > strings.Index(line, "REPO") || strings.Index(line, "REPO") > strings.Index(line, "Opus") {
		t.Errorf("order not kept: %q", line)
	}
}
