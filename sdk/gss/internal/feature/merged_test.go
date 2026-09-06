package feature_test

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// prNum extracts the trailing number from a .../pull/<n> URL.
func prNum(url string) int {
	i := strings.LastIndex(url, "/")
	n, _ := strconv.Atoi(url[i+1:])
	return n
}

// mw builds a worker row with a draft PR.
func mw(purpose, branch, base, prurl string, restack int) registry.Worker {
	return registry.Worker{
		User: "erai", Purpose: purpose, Branch: branch, Worktree: "/wt/" + purpose,
		BaseBranch: base, Description: purpose, PRURL: prurl, PRState: "draft", RestackCount: restack,
	}
}

// mergedService seeds a single feature (default base main) with the given
// workers and a gh fake holding a draft PR per worker that has a PRURL.
func mergedService(t *testing.T, workers []registry.Worker) (*feature.Service, *registry.Store, *ghfake.Client) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main", Workers: workers,
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ghc := ghfake.NewClient()
	for _, w := range workers {
		if w.PRURL != "" {
			ghc.SeedPR(gh.PR{Number: prNum(w.PRURL), Head: w.Branch, State: "OPEN", IsDraft: true, URL: w.PRURL})
		}
	}
	return &feature.Service{Store: store, GH: ghc}, store, ghc
}

func TestMergedLinearStackPromotesChild(t *testing.T) {
	svc, store, ghc := mergedService(t, []registry.Worker{
		mw("api", "feature/auth/erai/api", "main", "https://github.com/o/r/pull/42", 0),
		mw("ui", "feature/auth/erai/ui", "feature/auth/erai/api", "https://github.com/o/r/pull/43", 0),
	})
	res, err := svc.Merged(context.Background(), feature.MergedOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Merged: %v", err)
	}
	if res.Promoted != "auth/erai/ui" {
		t.Errorf("promoted = %q; want auth/erai/ui", res.Promoted)
	}
	// Ordering rule: re-target (PREdit base) on #43 must precede its promote (PRReady).
	calls := ghc.Calls()
	edit, ready := -1, -1
	for i, c := range calls {
		if c.Verb == ghfake.VerbPREdit && c.Num == 43 {
			edit = i
		}
		if c.Verb == ghfake.VerbPRReady && c.Num == 43 {
			ready = i
		}
	}
	if edit < 0 || ready < 0 || edit > ready {
		t.Errorf("want PREdit(#43) before PRReady(#43); got edit=%d ready=%d calls=%+v", edit, ready, calls)
	}
	reg, _ := store.Load()
	if got := reg.Features[0].Workers[0].PRState; got != "merged" {
		t.Errorf("api pr_state = %q; want merged", got)
	}
	ui := reg.Features[0].Workers[1]
	if ui.BaseBranch != "main" {
		t.Errorf("ui base = %q; want main (collapsed level)", ui.BaseBranch)
	}
	if ui.PRState != "open" {
		t.Errorf("ui pr_state = %q; want open (promoted)", ui.PRState)
	}
}

func TestMergedFanoutDoesNotPromote(t *testing.T) {
	svc, store, ghc := mergedService(t, []registry.Worker{
		mw("api", "feature/auth/erai/api", "main", "https://github.com/o/r/pull/42", 0),
		mw("ui", "feature/auth/erai/ui", "feature/auth/erai/api", "https://github.com/o/r/pull/43", 0),
		mw("docs", "feature/auth/erai/docs", "feature/auth/erai/api", "https://github.com/o/r/pull/44", 0),
	})
	res, err := svc.Merged(context.Background(), feature.MergedOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Merged: %v", err)
	}
	if res.Promoted != "" {
		t.Errorf("fan-out must not promote; got %q", res.Promoted)
	}
	if !strings.Contains(res.Notice, "fan-out") {
		t.Errorf("want a fan-out disqualification notice; got %q", res.Notice)
	}
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady {
			t.Errorf("fan-out: unexpected promote %+v", c)
		}
	}
	// Both children mechanically re-targeted onto main.
	reg, _ := store.Load()
	for _, w := range reg.Features[0].Workers[1:] {
		if w.BaseBranch != "main" {
			t.Errorf("%s base = %q; want main", w.Purpose, w.BaseBranch)
		}
	}
}

func TestMergedRestackedChildDoesNotPromote(t *testing.T) {
	svc, _, ghc := mergedService(t, []registry.Worker{
		mw("api", "feature/auth/erai/api", "main", "https://github.com/o/r/pull/42", 0),
		mw("ui", "feature/auth/erai/ui", "feature/auth/erai/api", "https://github.com/o/r/pull/43", 2),
	})
	res, err := svc.Merged(context.Background(), feature.MergedOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Merged: %v", err)
	}
	if res.Promoted != "" {
		t.Errorf("restacked child must not promote; got %q", res.Promoted)
	}
	if !strings.Contains(res.Notice, "restacked") {
		t.Errorf("want a restacked disqualification notice; got %q", res.Notice)
	}
	var edited, readied bool
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPREdit && c.Num == 43 {
			edited = true
		}
		if c.Verb == ghfake.VerbPRReady {
			readied = true
		}
	}
	if !edited {
		t.Error("child should still be mechanically re-targeted")
	}
	if readied {
		t.Error("restacked child must not be promoted")
	}
}

func TestMergedCascadesTwoLevels(t *testing.T) {
	svc, store, ghc := mergedService(t, []registry.Worker{
		mw("api", "feature/auth/erai/api", "main", "https://github.com/o/r/pull/42", 0),
		mw("ui", "feature/auth/erai/ui", "feature/auth/erai/api", "https://github.com/o/r/pull/43", 0),
		mw("docs", "feature/auth/erai/docs", "feature/auth/erai/ui", "https://github.com/o/r/pull/44", 0),
	})
	ctx := context.Background()

	// Level 1: api merges -> ui promoted; grandchild docs stays draft, base unchanged.
	r1, err := svc.Merged(ctx, feature.MergedOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Merged api: %v", err)
	}
	if r1.Promoted != "auth/erai/ui" {
		t.Fatalf("level1 promoted = %q; want auth/erai/ui", r1.Promoted)
	}
	reg, _ := store.Load()
	if got := reg.Features[0].Workers[2].PRState; got == "open" {
		t.Error("grandchild docs must stay draft after a level-1 merge")
	}
	if got := reg.Features[0].Workers[2].BaseBranch; got != "feature/auth/erai/ui" {
		t.Errorf("docs base = %q; want unchanged at level 1", got)
	}

	// Level 2: ui (now bottom, base main) merges -> docs promoted.
	r2, err := svc.Merged(ctx, feature.MergedOpts{WorkerRef: "auth/erai/ui"})
	if err != nil {
		t.Fatalf("Merged ui: %v", err)
	}
	if r2.Promoted != "auth/erai/docs" {
		t.Fatalf("level2 promoted = %q; want auth/erai/docs", r2.Promoted)
	}

	var readied []int
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady {
			readied = append(readied, c.Num)
		}
	}
	if len(readied) != 2 || readied[0] != 43 || readied[1] != 44 {
		t.Errorf("cascade PRReady order = %v; want [43 44]", readied)
	}
}
