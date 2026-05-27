// Package preview provides the bubbletea TUI model for interactive preview of
// the gsl status line, plus fixture payloads used by both the TUI and the
// --once golden output.
package preview

import (
	"time"

	gitfake "github.com/wenlock/dotfiles/gsl/internal/git/fake"
	"github.com/wenlock/dotfiles/gsl/internal/payload"
)

// strptr is a helper to make *string from a string literal.
func strptr(s string) *string { return &s }

// f64ptr is a helper to make *float64 from a float64 literal.
func f64ptr(v float64) *float64 { return &v }

// FixedClock returns a deterministic clock pinned to 2026-05-26 10:00:00 UTC,
// used for the --once golden output and preview model tests.
func FixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}
}

// CleanRepoPayload returns a representative payload for a clean git repo with
// Claude agent active (context at 30%).
func CleanRepoPayload() payload.Payload {
	return payload.Payload{
		Cwd:   strptr("/home/user/myproject"),
		Model: &payload.Model{DisplayName: strptr("Sonnet 4.6")},
		ContextWindow: &payload.ContextWindow{
			UsedPercentage:    f64ptr(30),
			TotalInputTokens:  f64ptr(60000),
			ContextWindowSize: f64ptr(200000),
		},
		RateLimits: &payload.RateLimits{
			FiveHour: &payload.RateWindow{UsedPercentage: f64ptr(10)},
		},
	}
}

// DirtyRepoPayload returns a representative payload for a dirty worktree with
// heavy context usage.
func DirtyRepoPayload() payload.Payload {
	return payload.Payload{
		Cwd:   strptr("/home/user/myproject/.worktrees/feature-x"),
		Model: &payload.Model{DisplayName: strptr("Opus 4.7")},
		ContextWindow: &payload.ContextWindow{
			UsedPercentage:    f64ptr(85),
			TotalInputTokens:  f64ptr(170000),
			ContextWindowSize: f64ptr(200000),
		},
		RateLimits: &payload.RateLimits{
			FiveHour: &payload.RateWindow{UsedPercentage: f64ptr(72)},
			SevenDay: &payload.RateWindow{UsedPercentage: f64ptr(45)},
		},
	}
}

// CleanGitResponses returns scripted git.Status responses for a clean repo on
// branch "main" (no staged/unstaged/untracked, 0 ahead/behind).
func CleanGitResponses() []gitfake.Response {
	porcelain := "# branch.oid abc123\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +0 -0\n"
	return []gitfake.Response{
		{Stdout: []byte(porcelain)},
		{Stdout: []byte("")}, // stash list: empty
	}
}

// DirtyGitResponses returns scripted git.Status responses for a dirty worktree
// on branch "feature-x" with staged, unstaged, and untracked files.
func DirtyGitResponses() []gitfake.Response {
	porcelain := "# branch.oid def456\n" +
		"# branch.head feature-x\n" +
		"# branch.upstream origin/feature-x\n" +
		"# branch.ab +3 -1\n" +
		"1 M. N... 100644 100644 100644 aaa bbb staged.go\n" +
		"1 .M N... 100644 100644 100644 ccc ddd dirty.go\n" +
		"? newfile.go\n"
	return []gitfake.Response{
		{Stdout: []byte(porcelain)},
		{Stdout: []byte("stash@{0}: WIP\n")}, // 1 stash
	}
}
