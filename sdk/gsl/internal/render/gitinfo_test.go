package render

// gitinfo_test.go — WS3 / F12: the pre-threaded git.Info.
//
// cmd/statusline.go ran git.Status SERIALLY (to get the branch) and then
// DirGitSegment ran git.Status AGAIN inside Detect: 4 git execs where 2 suffice,
// with the serial pair draining the shared 1s budget before the concurrent
// segments even start. Deps.GitInfo (the frozen interface) carries the already
// computed status into the segment.

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
)

// dirgitOnlyConfig is a config with ONLY the dirgit segment enabled, so the
// git-exec count attributable to that segment is unambiguous.
func dirgitOnlyConfig() config.Config {
	cfg := config.Default()
	cfg.Segments = []config.Segment{{Type: "dirgit", Enabled: true}}
	return cfg
}

// TestDetect_ReusesPreThreadedGitInfo is the subprocess-count assertion: with
// Deps.GitInfo pre-computed, the dirgit segment must spawn ZERO further git
// processes — and must still render the branch it was handed.
func TestDetect_ReusesPreThreadedGitInfo(t *testing.T) {
	spy := &gitfake.Runner{Script: gitStatusResponses("main")}

	deps := Deps{
		Cwd: "/tmp/myrepo",
		Git: spy,
		GitInfo: &git.Info{
			Branch:    "feature/pre-threaded",
			Staged:    1,
			Untracked: 2,
		},
	}

	cfg := dirgitOnlyConfig()
	st := asciiStyle()
	datas := Detect(context.Background(), cfg, st, BuildSegments(cfg, deps))

	if got := spy.CallCount(); got != 0 {
		t.Errorf("git.Runner.CallCount() = %d; want 0 — DirGitSegment re-ran git.Status "+
			"even though Deps.GitInfo was pre-computed (2 redundant execs per render; WS3/F12)", got)
	}

	out := Format(datas, st, 0)
	if !strings.Contains(out, "feature/pre-threaded") {
		t.Errorf("rendered line = %q; want it to contain the pre-threaded branch "+
			"'feature/pre-threaded' (the pre-computed Info must actually be USED)", out)
	}
}

// TestDetect_DetectsGitInfoWhenNotPreThreaded is the other half of the contract:
// a nil GitInfo means "not pre-computed; detect it yourself", which is what keeps
// every existing caller (tests, internal/preview) working unchanged.
func TestDetect_DetectsGitInfoWhenNotPreThreaded(t *testing.T) {
	spy := &gitfake.Runner{Script: gitStatusResponses("main")}

	deps := Deps{Cwd: "/tmp/myrepo", Git: spy, GitInfo: nil}

	cfg := dirgitOnlyConfig()
	st := asciiStyle()
	datas := Detect(context.Background(), cfg, st, BuildSegments(cfg, deps))

	// git.Status = `git status --porcelain=v2 --branch` + `git stash list` = 2 execs.
	if got := spy.CallCount(); got != 2 {
		t.Errorf("git.Runner.CallCount() = %d; want 2 (status + stash list) when GitInfo is nil", got)
	}
	if out := Format(datas, st, 0); !strings.Contains(out, "main") {
		t.Errorf("rendered line = %q; want it to contain the detected branch 'main'", out)
	}
}
