package render

import (
	"encoding/json"
	"time"

	gitfake "github.com/wenlock/dotfiles/gsl/internal/git/fake"
	"github.com/wenlock/dotfiles/gsl/internal/payload"
	"github.com/wenlock/dotfiles/gsl/internal/style"
)

// ── style fixtures ────────────────────────────────────────────────────────────

// discardWriter swallows Resolve's diagnostic warnings in tests.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// powerlineStyleFixture returns the resolved powerline (nerdfont) style.
func powerlineStyleFixture() style.Style {
	return style.Resolve(discardWriter{}, "powerline", nil, false)
}

// emojiStyleFixture returns the resolved emoji style.
func emojiStyleFixture() style.Style {
	return style.Resolve(discardWriter{}, "emoji", nil, false)
}

// asciiStyle returns the powerline style forced into ASCII glyphs.
func asciiStyle() style.Style {
	return style.Resolve(discardWriter{}, "powerline", nil, true)
}

// ── shared test fixtures ──────────────────────────────────────────────────────

// fixedClock returns a deterministic clock for golden/time tests:
// 2026-05-25 14:30:00 UTC. In America/Los_Angeles that is 07:30 PDT.
func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	}
}

// strptr / f64ptr are pointer constructors for building payload fixtures.
func strptr(s string) *string   { return &s }
func f64ptr(v float64) *float64 { return &v }

// samplePayload returns a fully-populated Claude payload fixture used by the
// AI-segment and golden tests.
func samplePayload() payload.Payload {
	return payload.Payload{
		Cwd:   strptr("/home/user/project"),
		Model: &payload.Model{DisplayName: strptr("Opus 4.7")},
		ContextWindow: &payload.ContextWindow{
			UsedPercentage:    f64ptr(42),
			TotalInputTokens:  f64ptr(84000),
			ContextWindowSize: f64ptr(200000),
		},
		RateLimits: &payload.RateLimits{
			FiveHour: &payload.RateWindow{UsedPercentage: f64ptr(80), ResetsAt: json.RawMessage("1779863400")},
			SevenDay: &payload.RateWindow{UsedPercentage: f64ptr(15)},
		},
	}
}

// gitStatusResponses returns the two scripted responses git.Status consumes:
//
//  1. `git status --porcelain=v2 --branch`
//  2. `git stash list`
//
// The porcelain block declares the given branch and a small set of changes
// (1 staged, 1 unstaged, 1 untracked) plus ahead=2/behind=0, and one stash.
func gitStatusResponses(branch string) []gitfake.Response {
	porcelain := "" +
		"# branch.oid abc123\n" +
		"# branch.head " + branch + "\n" +
		"# branch.upstream origin/" + branch + "\n" +
		"# branch.ab +2 -0\n" +
		"1 M. N... 100644 100644 100644 aaa bbb staged.go\n" +
		"1 .M N... 100644 100644 100644 ccc ddd unstaged.go\n" +
		"? untracked.go\n"
	return []gitfake.Response{
		{Stdout: []byte(porcelain)},
		{Stdout: []byte("stash@{0}: WIP on main\n")},
	}
}

// locateResponses returns the four scripted responses repo.Locate consumes:
//
//  1. rev-parse --git-dir
//  2. rev-parse --git-common-dir
//  3. rev-parse --show-toplevel
//  4. worktree list --porcelain
//
// isWorktree controls whether git-dir differs from git-common-dir (linked
// worktree) and how many worktree entries are listed.
func locateResponses(isWorktree bool, toplevel string, worktreeCount int) []gitfake.Response {
	gitDir := "/repo/.git"
	commonDir := "/repo/.git"
	if isWorktree {
		gitDir = "/repo/.git/worktrees/feat"
	}
	var wtList string
	for i := 0; i < worktreeCount; i++ {
		wtList += "worktree /path/" + string(rune('a'+i)) + "\nHEAD abc\nbranch refs/heads/x\n\n"
	}
	return []gitfake.Response{
		{Stdout: []byte(gitDir + "\n")},
		{Stdout: []byte(commonDir + "\n")},
		{Stdout: []byte(toplevel + "\n")},
		{Stdout: []byte(wtList)},
	}
}
