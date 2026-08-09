package render

// fit_property_test.go — the WS1 property suite (spec gsl-ultra §5: E3, E4, E5, E6).
//
// These are PROPERTY tests, not examples. The shipped suite asserted Fit's width
// invariant at four hand-picked column counts (80/60/40/30) and at cols=20 for two
// styles. That is exactly the shape of test that lets a real defect ship: the
// invariant is universally quantified ("for ALL inputs") but was only ever spot-checked.
//
// The matrix here sweeps {5 content fixtures} × {3 styles} × {worktree y/n} × {cols 1..200}
// and asserts the invariant on every single combination.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// ─── fixtures ─────────────────────────────────────────────────────────────────

// fitFixture describes one content scenario. The segmentData values are built
// DIRECTLY (no Detect, no fakes, no I/O) — Fit and Format are pure, so the
// property suite can be exhaustive and still run in well under a second.
type fitFixture struct {
	name   string
	cwd    string
	home   string
	branch string
	label  string // repo feature/worker label
	model  string
}

// fitFixtures is the content axis of the matrix.
func fitFixtures() []fitFixture {
	return []fitFixture{
		{
			name:   "short",
			cwd:    "/home/user/proj",
			home:   "/home/user",
			branch: "main",
			label:  "proj",
			model:  "Opus 4.7",
		},
		{
			name:   "long-basename",
			cwd:    "/home/user/git/gsl-ultra-tui-mcp-status",
			home:   "/home/user",
			branch: "main",
			label:  "gsl-ultra-tui-mcp-status",
			model:  "Claude Opus 4.8 (1M context)",
		},
		{
			name:   "long-branch",
			cwd:    "/home/user/git/dotfiles",
			home:   "/home/user",
			branch: "feature/gsl-ultra/edward-raigosa/width-and-fit-correctness",
			label:  "gsl-ultra",
			model:  "Claude Sonnet 4.5",
		},
		{
			// East-Asian wide runes: every rune is 2 display columns, so a
			// byte- or rune-count truncation silently overshoots the budget.
			name:   "cjk-path",
			cwd:    "/home/user/项目/文档目录内容",
			home:   "/home/user",
			branch: "主分支/开发/宽度修复",
			label:  "项目文档",
			model:  "Gemini 2.5 Pro",
		},
		{
			// A ZWJ emoji sequence is ONE grapheme cluster made of several runes.
			// Cutting between them produces mojibake AND a wrong width.
			name:   "emoji-branch",
			cwd:    "/home/user/git/rocket",
			home:   "/home/user",
			branch: "feat/🚀-launch-👩‍🚀-crew",
			label:  "rocket-🚀",
			model:  "Claude Haiku 4.5 (fast)",
		},
	}
}

// fitStyles is the presentation axis of the matrix.
func fitStyles() map[string]style.Style {
	return map[string]style.Style{
		"powerline": powerlineStyleFixture(),
		"emoji":     emojiStyleFixture(),
		"ascii":     asciiStyle(),
	}
}

// propDirGit builds a fully-populated dirGitData for the fixture.
func propDirGit(f fitFixture) segmentData {
	return &dirGitData{
		cwd:  f.cwd,
		home: f.home,
		gitInfo: &git.Info{
			Branch:    f.branch,
			Staged:    1,
			Unstaged:  2,
			Untracked: 3,
			Ahead:     2,
			Behind:    1,
			Stashes:   1,
		},
		hasGit: true,
	}
}

// propRepo builds a fully-populated repoData for the fixture.
func propRepo(f fitFixture, worktree bool) segmentData {
	key := "repo_root"
	count := 0
	if worktree {
		key = "repo_worktree"
		count = 4
	}
	return &repoData{
		themeKey:      key,
		indicatorKey:  key,
		label:         f.label,
		prNumber:      157,
		prState:       "OPEN",
		worktreeCount: count,
		showPR:        true,
		showCount:     worktree,
	}
}

// propAI builds a fully-populated aiData for the fixture.
func propAI(f fitFixture) segmentData {
	return &aiData{
		modelName:     f.model,
		ctxPct:        f64ptr(42),
		tokenUsed:     f64ptr(84000),
		tokenTotal:    f64ptr(200000),
		mcpActive:     6,
		mcpConfigured: 12,
		rate5h:        f64ptr(80),
		rate7d:        f64ptr(15),
		hasPayload:    true,
	}
}

// propTime builds a fully-populated timeData.
func propTime() segmentData {
	return &timeData{
		t:          time.Date(2026, 7, 11, 14, 30, 5, 0, time.UTC),
		dateLayout: "2006-01-02",
		timeLayout: "15:04:05",
		tz:         "UTC",
	}
}

// propDatas returns the four segmentData values in canonical config order
// (dirgit, repo, ai, time) for the fixture.
func propDatas(f fitFixture, worktree bool) []segmentData {
	return []segmentData{
		propDirGit(f),
		propRepo(f, worktree),
		propAI(f),
		propTime(),
	}
}

// ─── E3 — the width invariant ─────────────────────────────────────────────────

// TestFit_WidthInvariant is spec rule E3.
//
//	term.DisplayWidth(Fit(datas, st, cols)) <= cols   for ALL inputs.
//
// This must hold for EVERY combination of content, style, worktree state and
// column count in 1..200 — no exceptions, no "except when very narrow".
func TestFit_WidthInvariant(t *testing.T) {
	styles := fitStyles()
	for _, f := range fitFixtures() {
		for styleName, st := range styles {
			for _, worktree := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/worktree=%v", f.name, styleName, worktree)
				t.Run(name, func(t *testing.T) {
					datas := propDatas(f, worktree)
					for cols := 1; cols <= 200; cols++ {
						out := Fit(datas, st, cols)
						if w := term.DisplayWidth(out); w > cols {
							t.Fatalf("E3 VIOLATED: cols=%d → DisplayWidth=%d\n  output: %q",
								cols, w, out)
						}
					}
				})
			}
		}
	}
}

// ─── E4 — Fit must emit something ─────────────────────────────────────────────

// TestFit_NonEmpty is spec rule E4: Fit must return non-empty output whenever
// at least one segmentData is non-nil and has content. Fitting to a narrow
// terminal degrades the line; it must never delete it.
func TestFit_NonEmpty(t *testing.T) {
	styles := fitStyles()
	for _, f := range fitFixtures() {
		for styleName, st := range styles {
			for _, worktree := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/worktree=%v", f.name, styleName, worktree)
				t.Run(name, func(t *testing.T) {
					datas := propDatas(f, worktree)
					for cols := 1; cols <= 200; cols++ {
						if out := Fit(datas, st, cols); out == "" {
							t.Fatalf("E4 VIOLATED: cols=%d → empty output with %d live segments",
								cols, len(datas))
						}
					}
				})
			}
		}
	}

	// A single live segment (the rest nil) must still render.
	st := emojiStyleFixture()
	f := fitFixtures()[1]
	for i := 0; i < 4; i++ {
		datas := make([]segmentData, 4)
		datas[i] = propDatas(f, false)[i]
		for cols := 1; cols <= 200; cols++ {
			if out := Fit(datas, st, cols); out == "" {
				t.Fatalf("E4 VIOLATED: lone segment idx=%d, cols=%d → empty output", i, cols)
			}
		}
	}
}

// ─── E5 — per-level width monotonicity ────────────────────────────────────────

// TestSegmentWidth_MonotonicInLevel is spec rule E5.
//
// For EVERY segmentData type, DisplayWidth(format(st, level)) must be
// NON-INCREASING as level goes 0 → 4. A segment whose width is flat across all
// levels is not compacting — it is silently defeating the entire Fit ladder,
// which is what forces Fit into the segment-drop tier far earlier than it should.
//
// This is the test that catches repoData ignoring its level parameter.
func TestSegmentWidth_MonotonicInLevel(t *testing.T) {
	styles := fitStyles()
	for _, f := range fitFixtures() {
		for styleName, st := range styles {
			for _, worktree := range []bool{false, true} {
				types := map[string]segmentData{
					"dirGitData": propDirGit(f),
					"repoData":   propRepo(f, worktree),
					"aiData":     propAI(f),
					"timeData":   propTime(),
				}
				for typeName, d := range types {
					name := fmt.Sprintf("%s/%s/%s/worktree=%v", typeName, f.name, styleName, worktree)
					t.Run(name, func(t *testing.T) {
						widths := make([]int, finalCompactLevel+1)
						texts := make([]string, finalCompactLevel+1)
						for level := 0; level <= finalCompactLevel; level++ {
							text, _ := d.format(st, level)
							texts[level] = text
							widths[level] = term.DisplayWidth(text)
						}
						for level := 1; level <= finalCompactLevel; level++ {
							if widths[level] > widths[level-1] {
								t.Fatalf("E5 VIOLATED (grew): level %d width %d > level %d width %d\n  L%d: %q\n  L%d: %q",
									level, widths[level], level-1, widths[level-1],
									level-1, texts[level-1], level, texts[level])
							}
						}
						// A segment that never gets narrower is not compacting at all.
						// (Only assert this when there IS something to compact: the
						// level-0 render is wider than a bare glyph + a few columns.)
						if widths[0] >= 12 && widths[finalCompactLevel] == widths[0] {
							t.Fatalf("E5 VIOLATED (flat): %s width is %d at EVERY level 0..%d — the level parameter is being ignored\n  L0: %q\n  L%d: %q",
								typeName, widths[0], finalCompactLevel,
								texts[0], finalCompactLevel, texts[finalCompactLevel])
						}
					})
				}
			}
		}
	}
}

// ─── E6 — drop order must not follow slice position ───────────────────────────

// TestFit_DropOrderIndependentOfConfigOrder is spec rule E6.
//
// Fit drops segments when text compaction is not enough. WHICH segment survives
// must be decided by an explicit PRIORITY, not by where the user happened to put
// the segment in cfg.Segments. Reversing the config order must not change the
// survivor.
//
// Today Fit does `active = active[:len(active)-1]` — it drops the RIGHTMOST slice
// element. So the survivor is simply cfg.Segments[0], and reversing the config
// changes the answer.
func TestFit_DropOrderIndependentOfConfigOrder(t *testing.T) {
	st := emojiStyleFixture()

	// Four minimal segments with 2-column marker payloads, so that at the final
	// tier every block renders to exactly its 2-column marker.
	mk := func() []segmentData {
		return []segmentData{
			&dirGitData{cwd: "/x/Da", home: "/x"},                                                          // "Da"
			&repoData{themeKey: "repo_root", indicatorKey: "repo_root", label: "Rb"},                       // "Rb"
			&aiData{hasPayload: true, modelName: "Ac", mcpActive: -1},                                      // "Ac"
			&timeData{t: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), timeLayout: "Td", dateLayout: "Xy"}, // "Td"
		}
	}
	const (
		markerDirGit = "Da"
		markerRepo   = "Rb"
		markerAI     = "Ac"
		markerTime   = "Td"
	)

	forward := mk()
	reversed := mk()
	// Reverse the slice — this models the user reordering cfg.Segments.
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	// cols = exactly the width of a lone repo block at the final tier, so exactly
	// one segment can survive. Derived, not hardcoded.
	repoBlockText, repoKey := mk()[1].format(st, finalCompactLevel-1)
	repoBlockText = dropLeadingGlyph(repoBlockText)
	cols := term.DisplayWidth(join(st, []segmentBlock{{text: repoBlockText, colorKey: repoKey}}))

	gotFwd := Fit(forward, st, cols)
	gotRev := Fit(reversed, st, cols)

	survivors := func(out string) []string {
		var s []string
		for _, m := range []string{markerDirGit, markerRepo, markerAI, markerTime} {
			if strings.Contains(out, m) {
				s = append(s, m)
			}
		}
		return s
	}

	fwdS, revS := survivors(gotFwd), survivors(gotRev)

	if len(fwdS) != 1 || fwdS[0] != markerRepo {
		t.Errorf("E6 VIOLATED (forward order): cols=%d → survivors %v, want [%s]\n  output: %q",
			cols, fwdS, markerRepo, gotFwd)
	}
	if len(revS) != 1 || revS[0] != markerRepo {
		t.Errorf("E6 VIOLATED (reversed order): cols=%d → survivors %v, want [%s]\n  output: %q",
			cols, revS, markerRepo, gotRev)
	}
	if strings.Join(fwdS, ",") != strings.Join(revS, ",") {
		t.Errorf("E6 VIOLATED: drop order follows slice position — forward survivors %v != reversed survivors %v",
			fwdS, revS)
	}
}

// TestBuildSegments_ThreadsPriority closes the loop from config.json to the fit
// loop: a Priority set in the config must reach the constructed segment, which
// stamps it onto the segmentData, which is what Fit sorts by. Without this the
// E6 property above would pass on the built-in defaults while a user's explicit
// priority silently did nothing.
func TestBuildSegments_ThreadsPriority(t *testing.T) {
	cfg := config.Default()
	cfg.Segments = []config.Segment{
		{Type: "time", Enabled: true, Priority: 99}, // explicitly promoted
		{Type: "repo", Enabled: true},               // default
		{Type: "ai", Enabled: false},                // disabled → skipped
	}

	segs := BuildSegments(cfg, Deps{})
	if len(segs) != 2 {
		t.Fatalf("want 2 enabled segments, got %d", len(segs))
	}

	ts, ok := segs[0].(*TimeSegment)
	if !ok {
		t.Fatalf("segs[0] is %T, want *TimeSegment", segs[0])
	}
	if ts.Priority != 99 {
		t.Errorf("TimeSegment.Priority = %d, want the config's explicit 99", ts.Priority)
	}

	rs, ok := segs[1].(*RepoSegment)
	if !ok {
		t.Fatalf("segs[1] is %T, want *RepoSegment", segs[1])
	}
	if rs.Priority != config.PriorityRepo {
		t.Errorf("RepoSegment.Priority = %d, want the built-in default %d", rs.Priority, config.PriorityRepo)
	}
}

// TestFit_ExplicitPriorityOverridesDefault proves the user's priority actually
// changes WHICH segment survives — the time segment, normally the first thing
// dropped, outlives the repo segment when the user says it should.
func TestFit_ExplicitPriorityOverridesDefault(t *testing.T) {
	st := emojiStyleFixture()

	datas := []segmentData{
		&repoData{themeKey: "repo_root", indicatorKey: "repo_root", label: "Rb"}, // default: PriorityRepo (highest)
		&timeData{
			t:          time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
			timeLayout: "Td",
			dateLayout: "Xy",
			prio:       99, // the user promoted the clock above everything
		},
	}

	out := Fit(datas, st, 2) // room for exactly one 2-column block
	if !strings.Contains(out, "Td") {
		t.Errorf("explicit priority ignored: want the promoted time segment to survive, got %q", out)
	}
	if strings.Contains(out, "Rb") {
		t.Errorf("want the repo segment dropped despite its higher DEFAULT priority, got %q", out)
	}
}
