package render

// Tests for the Detect/Format split (Phase 2 — detect-once / format-per-level).
//
// These tests cover:
//  1. A counting fake runner that proves detection runs EXACTLY ONCE across a
//     full Fit call that exercises multiple compaction levels.
//  2. Per-level table tests for ai, time, and dirgit formatting.
//  3. The final tier (glyph-drop then segment-drop).
//  4. Fit() — first output whose DisplayWidth ≤ cols; wide-cols case returns
//     level-0 output unchanged.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	mcpfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// ─── counting fake git runner ─────────────────────────────────────────────────

// countingGitRunner wraps gitfake.Runner and counts every Run call.
type countingGitRunner struct {
	inner *gitfake.Runner
	calls atomic.Int32
}

func (r *countingGitRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls.Add(1)
	return r.inner.Run(ctx, name, args...)
}

// ─── TestDetect_RunsOnce ─────────────────────────────────────────────────────

// TestDetect_RunsOnce proves that calling Detect ONCE and then Fit/Format at
// multiple levels does NOT re-run the git/mcp subprocess. The counting runner's
// call count is read after Fit completes; it must equal the count from Detect
// alone.
func TestDetect_RunsOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	// Wire enough git responses for one full dirgit+repo detect pass.
	dirGitResponses := gitStatusResponses("feature")
	repoResponses := locateResponses(false, "/wt/proj", 1)
	allResponses := append(dirGitResponses, repoResponses...)

	cg := &countingGitRunner{
		inner: &gitfake.Runner{Script: allResponses},
	}

	writeMcpJSON(t, dir, 2)
	mcpRunner := &mcpfake.Runner{Default: mcpfake.Response{
		Stdout: []byte("a: x - ✓ Connected\nb: y - ✗ no\n"),
	}}

	st := emojiStyleFixture()
	cfg := config.Default()

	deps := Deps{
		Payload:      samplePayload(),
		Cwd:          dir,
		Branch:       "feature",
		RegistryPath: registryPath(),
		Git:          cg,
		MCP:          mcpRunner,
		MCPOpts:      mcp.ActiveCountOptions{CacheFile: filepath.Join(dir, "ac.json")},
		Clock:        fixedClock(),
	}
	segs := BuildSegments(cfg, deps)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	callsAfterDetect := cg.calls.Load()
	if callsAfterDetect == 0 {
		t.Fatal("Detect: no git runner calls — something is wrong with the test setup")
	}

	// Now call Format at multiple levels — no additional runner calls expected.
	for level := 0; level <= 3; level++ {
		_ = Format(datas, st, level)
	}
	// Also call Fit.
	_ = Fit(datas, st, 20)

	callsAfterFit := cg.calls.Load()
	if callsAfterFit != callsAfterDetect {
		t.Errorf("detection-count: Detect used %d runner calls; Format+Fit added %d more (want 0 extra)",
			callsAfterDetect, callsAfterFit-callsAfterDetect)
	}
}

// ─── Format level table tests ──────────────────────────────────────────────────

// TestFormat_Level0_NonEmpty verifies that Format at level 0 returns a non-empty
// string when there are active segments.
func TestFormat_Level0_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := emojiStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	out := Format(datas, st, 0)
	if out == "" {
		t.Error("Format level 0: want non-empty output, got empty")
	}
}

// TestFormat_HigherLevel_ShorterOrEqual verifies that each successive level
// produces output that is ≤ in display width to the previous level.
func TestFormat_HigherLevel_ShorterOrEqual(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := emojiStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	prevWidth := term.DisplayWidth(Format(datas, st, 0))
	for level := 1; level <= finalCompactLevel; level++ {
		out := Format(datas, st, level)
		w := term.DisplayWidth(out)
		if w > prevWidth {
			t.Errorf("level %d is wider (%d) than level %d (%d) — compaction must never grow",
				level, w, level-1, prevWidth)
		}
		prevWidth = w
	}
}

// ─── AI segment per-level compaction ──────────────────────────────────────────

func TestAISegmentData_Levels(t *testing.T) {
	p := samplePayload()
	st := emojiStyleFixture()

	// Build one AISegment (no MCP to keep things simple).
	seg := NewAISegment(p, "", nil, mcp.ActiveCountOptions{})
	ctx := context.Background()

	data, ok := seg.detect(ctx)
	if !ok {
		t.Fatal("AISegment.detect: want ok=true")
	}

	// level 0: full — model + ctx% + token-ratio + MCP + rate limits
	l0, _ := data.format(st, 0)
	if !strings.Contains(l0, "Opus 4.7") {
		t.Errorf("AI level 0: missing model name in %q", l0)
	}
	if !strings.Contains(l0, "84k/200k") {
		t.Errorf("AI level 0: missing token ratio in %q", l0)
	}
	if !strings.Contains(l0, "7d") {
		t.Errorf("AI level 0: missing 7d rate in %q", l0)
	}

	// level 1: drop 7d rate limit
	l1, _ := data.format(st, 1)
	if strings.Contains(l1, "7d") {
		t.Errorf("AI level 1: 7d should be dropped, got %q", l1)
	}
	if !strings.Contains(l1, "84k/200k") {
		t.Errorf("AI level 1: token ratio should still be present in %q", l1)
	}

	// level 2: drop token ratio (keep percentage only)
	l2, _ := data.format(st, 2)
	if strings.Contains(l2, "84k/200k") {
		t.Errorf("AI level 2: token ratio should be dropped, got %q", l2)
	}
	if strings.Contains(l2, "7d") {
		t.Errorf("AI level 2: 7d should still be dropped, got %q", l2)
	}
	if !strings.Contains(l2, "Opus") {
		t.Errorf("AI level 2: model name should still be present, got %q", l2)
	}

	// level 3: shorten model name + drop MCP (model still recognizable but shorter)
	l3, _ := data.format(st, 3)
	// Level 3 shortens the model name — it should be shorter than the full name.
	if len(l3) >= len(l0) {
		t.Errorf("AI level 3: want shorter than level 0, got %q vs %q", l3, l0)
	}
}

// ─── Time segment per-level compaction ────────────────────────────────────────

func TestTimeSegmentData_Levels(t *testing.T) {
	seg := NewTimeSegment(fixedClock(), "UTC", "15:04:05", "Mon 01-02")
	ctx := context.Background()
	st := emojiStyleFixture()

	data, ok := seg.detect(ctx)
	if !ok {
		t.Fatal("TimeSegment.detect: want ok=true")
	}

	l0, _ := data.format(st, 0)
	// level 0 should have date, time (with seconds), and tz
	if !strings.Contains(l0, "Mon") || !strings.Contains(l0, "14:30") || !strings.Contains(l0, "UTC") {
		t.Errorf("time level 0: want date+time+tz, got %q", l0)
	}

	// level 1: drop date
	l1, _ := data.format(st, 1)
	if strings.Contains(l1, "Mon") {
		t.Errorf("time level 1: date should be dropped, got %q", l1)
	}
	if !strings.Contains(l1, "14:30") {
		t.Errorf("time level 1: time should still be present, got %q", l1)
	}

	// level 2: drop seconds (HH:MM only)
	l2, _ := data.format(st, 2)
	// With seconds dropped, "14:30:00" becomes "14:30"; no seconds component.
	if strings.Contains(l2, ":00") {
		t.Errorf("time level 2: seconds should be dropped, got %q", l2)
	}
	if strings.Contains(l2, "Mon") {
		t.Errorf("time level 2: date should still be dropped, got %q", l2)
	}

	// level 3: drop timezone
	l3, _ := data.format(st, 3)
	if strings.Contains(l3, "UTC") {
		t.Errorf("time level 3: tz should be dropped, got %q", l3)
	}
}

// ─── DirGit segment per-level compaction ─────────────────────────────────────

func TestDirGitSegmentData_Levels(t *testing.T) {
	// Use a deep path to test abbreviation.
	dirRunner := &gitfake.Runner{Script: gitStatusResponses("main")}
	seg := &DirGitSegment{
		Cwd:  "/home/user/projects/myrepo/subdir/deep",
		Git:  dirRunner,
		home: "/home/user",
	}
	ctx := context.Background()
	st := emojiStyleFixture()

	data, ok := seg.detect(ctx)
	if !ok {
		t.Fatal("DirGitSegment.detect: want ok=true")
	}

	l0, _ := data.format(st, 0)
	// level 0: basename of cwd + branch
	if !strings.Contains(l0, "deep") {
		t.Errorf("dirgit level 0: want cwd basename 'deep', got %q", l0)
	}
	if !strings.Contains(l0, "main") {
		t.Errorf("dirgit level 0: want branch 'main', got %q", l0)
	}

	// Test with a longer path where abbreviation triggers at level 1.
	dirRunner2 := &gitfake.Runner{Script: gitStatusResponses("feature/gsl/impl")}
	seg2 := &DirGitSegment{
		Cwd:  "/home/user/projects/a/b/c/d",
		Git:  dirRunner2,
		home: "/home/user",
	}
	data2, ok2 := seg2.detect(ctx)
	if !ok2 {
		t.Fatal("DirGitSegment.detect seg2: want ok=true")
	}

	l0_2, _ := data2.format(st, 0)
	l1_2, _ := data2.format(st, 1)
	l2_2, _ := data2.format(st, 2)
	l3_2, _ := data2.format(st, 3)

	// levels should be non-increasing in width
	if term.DisplayWidth(l1_2) > term.DisplayWidth(l0_2) {
		t.Errorf("dirgit level 1: want width ≤ level 0: %q vs %q", l1_2, l0_2)
	}
	if term.DisplayWidth(l2_2) > term.DisplayWidth(l1_2) {
		t.Errorf("dirgit level 2: want width ≤ level 1: %q vs %q", l2_2, l1_2)
	}
	if term.DisplayWidth(l3_2) > term.DisplayWidth(l2_2) {
		t.Errorf("dirgit level 3: want width ≤ level 2: %q vs %q", l3_2, l2_2)
	}
}

// ─── Fit loop ─────────────────────────────────────────────────────────────────

// TestFit_WideColsReturnsLevel0 verifies that when level-0 output fits, it is
// returned unchanged (no compaction).
func TestFit_WideColsReturnsLevel0(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := emojiStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	level0 := Format(datas, st, 0)
	// Use a very wide column count — output should equal level 0.
	got := Fit(datas, st, 500)
	if got != level0 {
		t.Errorf("Fit wide cols: want level-0 output unchanged\n got: %q\nwant: %q", got, level0)
	}
}

// TestFit_NarrowFits verifies that Fit returns output whose DisplayWidth ≤ cols.
func TestFit_NarrowFits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := emojiStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	for _, cols := range []int{80, 60, 40, 30} {
		t.Run(fmt.Sprintf("cols=%d", cols), func(t *testing.T) {
			out := Fit(datas, st, cols)
			w := term.DisplayWidth(out)
			if w > cols {
				t.Errorf("Fit cols=%d: output width %d > %d\noutput: %q", cols, w, cols, out)
			}
		})
	}
}

// TestFit_Emoji_COLUMNS20 is the binding case: emoji at COLUMNS=20.
// Because each emoji glyph is ~2 cols, text-trimming alone cannot fit; the
// final tier (glyph-drop then segment-drop) must kick in.
func TestFit_Emoji_COLUMNS20(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := emojiStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	out := Fit(datas, st, 20)
	w := term.DisplayWidth(out)
	if w > 20 {
		t.Errorf("Fit emoji COLUMNS=20: output width %d > 20\noutput: %q", w, out)
	}
	if out == "" {
		t.Error("Fit emoji COLUMNS=20: output must not be empty (some content should survive)")
	}
}

// TestFit_Powerline_COLUMNS20 verifies the powerline style also fits at 20 cols.
func TestFit_Powerline_COLUMNS20(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st := powerlineStyleFixture()
	cfg := config.Default()
	segs := buildGoldenSegments(t, false, dir)

	ctx := context.Background()
	datas := Detect(ctx, cfg, st, segs)

	out := Fit(datas, st, 20)
	w := term.DisplayWidth(out)
	if w > 20 {
		t.Errorf("Fit powerline COLUMNS=20: output width %d > 20\noutput: %q", w, out)
	}
}

// TestFormat_EmptyDatas verifies Format returns "" for nil/empty data.
func TestFormat_EmptyDatas(t *testing.T) {
	st := emojiStyleFixture()
	if got := Format(nil, st, 0); got != "" {
		t.Errorf("Format nil datas: want empty, got %q", got)
	}
}

// ─── Detect vs Render equivalence ─────────────────────────────────────────────

// TestDetectFormat_MatchesRender checks that Detect+Format(level=0) produces
// the same output as the legacy Render call (which internally does Detect+Format).
func TestDetectFormat_MatchesRender(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	for _, styleName := range []string{"emoji", "powerline"} {
		t.Run(styleName, func(t *testing.T) {
			st := style.Resolve(discardWriter{}, styleName, nil, false)
			cfg := config.Default()
			segs := buildGoldenSegments(t, false, dir)
			ctx := context.Background()

			// Legacy path.
			renderOut := Render(ctx, cfg, st, segs)
			// New path (fresh segments to avoid state sharing).
			segs2 := buildGoldenSegments(t, false, dir)
			datas := Detect(ctx, cfg, st, segs2)
			detectFormatOut := Format(datas, st, 0)

			if renderOut != detectFormatOut {
				t.Errorf("Detect+Format(0) != Render for %s\n"+
					" Render:      %q\n"+
					" Detect+Fmt:  %q", styleName, renderOut, detectFormatOut)
			}
		})
	}
}
