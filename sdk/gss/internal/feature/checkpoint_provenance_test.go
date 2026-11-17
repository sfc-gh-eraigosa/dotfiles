package feature_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/provenance"
)

// TestCheckpoint_WritesProvenanceSection: with Provenance set, the PR body
// gets one managed Provenance section, kept once across checkpoints; the
// prose and the stack block are untouched.
func TestCheckpoint_WritesProvenanceSection(t *testing.T) {
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 7, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true,
		URL: "https://github.com/o/r/pull/7", Body: "prose"})
	svc, _, _ := checkpointService(t, "https://github.com/o/r/pull/7", nil, ghc)
	svc.Git = &gitfake.Runner{} // every git call succeeds with no output
	svc.Provenance = &provenance.Info{Harness: "claude", Session: "s-1", HostID: "h-0123456789"}

	for i := 0; i < 2; i++ {
		if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
			t.Fatalf("Checkpoint %d: %v", i, err)
		}
		e := lastPREdit(ghc)
		ghc.SeedPR(gh.PR{Number: 7, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true,
			URL: "https://github.com/o/r/pull/7", Body: e.EditOpts.Body})
	}
	body := lastPREdit(ghc).EditOpts.Body
	for _, want := range []string{"prose", "## Provenance", "- Built by: claude", "`s-1`", "`h-0123456789`", "- Privacy hooks: ", "<!-- gss:stack-begin -->"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if n := strings.Count(body, "<!-- gss:provenance-begin -->"); n != 1 {
		t.Errorf("%d provenance sections after two checkpoints:\n%s", n, body)
	}
}

// TestCheckpoint_NoProvenanceWhenUnset pins the default for callers that do
// not set it (every other test here).
func TestCheckpoint_NoProvenanceWhenUnset(t *testing.T) {
	ghc := ghfake.NewClient()
	svc, _, _ := checkpointService(t, "", []gitfake.Response{{}, {}, {}}, ghc)
	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatal(err)
	}
	if c := lastPRCreate(ghc); c == nil || strings.Contains(c.CreateOpts.Body, "gss:provenance") {
		t.Errorf("provenance written without Service.Provenance: %+v", c)
	}
}
