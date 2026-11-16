package feature_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

const staleBody = "stale prose from round one\n\n<!-- gss:stack-begin -->\n## Stack\nold block\n<!-- gss:stack-end -->\n"

// existingPR seeds an open draft PR #7 for the api worker with a stale body.
func existingPR(t *testing.T) (*ghfake.Client, string) {
	t.Helper()
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 7, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true,
		URL: "https://github.com/o/r/pull/7", Body: staleBody})
	return ghc, "https://github.com/o/r/pull/7"
}

// TestCheckpoint_BodyFromReplacesProse: --body-from swaps the prose of an
// existing PR and keeps exactly one freshly rendered stack block.
func TestCheckpoint_BodyFromReplacesProse(t *testing.T) {
	ghc, url := existingPR(t)
	svc, _, _ := checkpointService(t, url, []gitfake.Response{{}, {}, {}}, ghc)

	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api", Body: "## What\n\nfresh prose\n"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	e := lastPREdit(ghc)
	if e == nil || e.Num != 7 {
		t.Fatalf("expected PREdit on #7; got %+v", e)
	}
	body := e.EditOpts.Body
	if !strings.Contains(body, "fresh prose") || strings.Contains(body, "stale prose") || strings.Contains(body, "old block") {
		t.Errorf("prose not replaced:\n%s", body)
	}
	if strings.Count(body, "<!-- gss:stack-begin -->") != 1 || !strings.Contains(body, "← you are here") {
		t.Errorf("want one re-rendered stack block:\n%s", body)
	}
}

// TestCheckpoint_RefreshBodyReseedsFromWorkerMD: --refresh-body reads
// WORKER.md again for a PR that already exists.
func TestCheckpoint_RefreshBodyReseedsFromWorkerMD(t *testing.T) {
	wt := filepath.Join(t.TempDir(), "auth", "erai", "api")
	plantWorkerMD(t, wt, seededWorkerMD)
	ghc, url := existingPR(t)
	svc, _, _ := checkpointServiceAt(t, wt, url, []gitfake.Response{{}, {}, {}}, ghc)

	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api", RefreshBody: true}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	body := lastPREdit(ghc).EditOpts.Body
	if !strings.Contains(body, "Ship the endpoints.") || strings.Contains(body, "stale prose") {
		t.Errorf("WORKER.md prose not reseeded:\n%s", body)
	}
}

// TestCheckpoint_WithoutRefreshKeepsProse pins the default: no flag, the
// PR's own text stays.
func TestCheckpoint_WithoutRefreshKeepsProse(t *testing.T) {
	ghc, url := existingPR(t)
	svc, _, _ := checkpointService(t, url, []gitfake.Response{{}, {}, {}}, ghc)
	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if body := lastPREdit(ghc).EditOpts.Body; !strings.Contains(body, "stale prose") {
		t.Errorf("default checkpoint dropped the PR prose:\n%s", body)
	}
}

// TestCheckpoint_BodyFlagsExclusive: both flags at once is refused before
// any git call.
func TestCheckpoint_BodyFlagsExclusive(t *testing.T) {
	ghc, url := existingPR(t)
	svc, _, gitr := checkpointService(t, url, nil, ghc)
	_, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api", Body: "x", RefreshBody: true})
	if err == nil || !strings.Contains(err.Error(), "exclusive") {
		t.Fatalf("err = %v; want an exclusive-flags error", err)
	}
	if len(gitr.Calls) != 0 {
		t.Errorf("git ran %d times before the refusal", len(gitr.Calls))
	}
}

// TestCheckpoint_StackRowsOmitUser: the stack block names workers by leaf
// and a stacked base by its PR number, so the user segment of a branch
// (the git login slug) never reaches a PR body.
func TestCheckpoint_StackRowsOmitUser(t *testing.T) {
	ghc, url := existingPR(t)
	svc, store, _ := checkpointService(t, url, []gitfake.Response{{}, {}, {}}, ghc)
	if err := store.Update(func(r *registry.Registry) error {
		r.Features[0].Workers = append(r.Features[0].Workers,
			registry.Worker{User: "erai", Purpose: "ui", Branch: "feature/auth/erai/ui",
				Worktree: "/wt/ui", BaseBranch: "feature/auth/erai/api"},
			registry.Worker{User: "erai", Purpose: "docs", Branch: "feature/auth/erai/docs",
				Worktree: "/wt/docs", BaseBranch: "feature/other/erai/x"},
			registry.Worker{User: "erai", Purpose: "deep", Branch: "feature/auth/erai/deep",
				Worktree: "/wt/deep", BaseBranch: "feature/other/erai/x/y"},
			registry.Worker{User: "erai", Purpose: "rel", Branch: "feature/auth/erai/rel",
				Worktree: "/wt/rel", BaseBranch: "release/erai/v1"},
			registry.Worker{User: "erai", Purpose: "solo", Branch: "feature/auth/erai/solo",
				Worktree: "/wt/solo", BaseBranch: "erai"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	body := lastPREdit(ghc).EditOpts.Body
	stackPart := body[strings.Index(body, "<!-- gss:stack-begin -->"):]
	if strings.Contains(stackPart, "erai") {
		t.Errorf("stack block names the user:\n%s", stackPart)
	}
	for _, want := range []string{"**#7 — api (base: `main`)**", "(no PR yet) — ui (base: `#7`)", "(no PR yet) — docs (base: `other/x`)", "(no PR yet) — deep (base: `other/x/y`)", "(no PR yet) — rel (base: `release/v1`)", "(no PR yet) — solo (base: `(a user branch)`)"} {
		if !strings.Contains(stackPart, want) {
			t.Errorf("stack block missing %q:\n%s", want, stackPart)
		}
	}
}
