// Package fake_test verifies the recording / scriptable gh.Client fake
// per sdk/gss/docs/plan.md PR-03.
//
// The fake is the contract downstream packages (internal/classic/pr.go,
// internal/feature/*, internal/stack/*) depend on for hermetic offline
// tests. Its observable behaviour is therefore part of the public
// contract and is covered here:
//
//   - It is STATEFUL: PRCreate mints a numbered PR, PRReady flips the
//     stored draft flag, PREdit rewrites the stored base/body, and PRView
//     / PRList read that evolving state back. This is what lets an
//     orchestrator test assert "create draft → promote → it's ready".
//   - Its error injection is PER-VERB (carry-forward note #9): scripting
//     an error on PRView does not consume the script for PRCreate. This
//     deliberately differs from the git fake's single global FIFO, because
//     gh callers interleave verbs and need to fail one in isolation.
package fake_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
)

// TestFakeClient_ImplementsClient — compile-time proof the fake is a drop-in
// gh.Client (carry-forward note #4).
func TestFakeClient_ImplementsClient(t *testing.T) {
	var _ gh.Client = (*ghfake.Client)(nil)
}

// TestFakeClient_CreateDraftThenReady — the headline stateful transition:
// create a draft, promote it, and confirm PRView observes IsDraft flip.
func TestFakeClient_CreateDraftThenReady(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()

	pr, err := c.PRCreate(ctx, gh.PRCreateOpts{
		Title: "PR-03", Base: "pr02", Head: "pr03", Draft: true,
	})
	if err != nil {
		t.Fatalf("PRCreate: %v", err)
	}
	if pr.Number == 0 || !pr.IsDraft || pr.State != "OPEN" {
		t.Fatalf("created PR wrong: %+v", pr)
	}

	if err := c.PRReady(ctx, pr.Number); err != nil {
		t.Fatalf("PRReady: %v", err)
	}

	got, err := c.PRView(ctx, pr.Number)
	if err != nil {
		t.Fatalf("PRView: %v", err)
	}
	if got.IsDraft {
		t.Errorf("after PRReady, PRView IsDraft = true; want false")
	}
}

// TestFakeClient_EditBaseAndBody — PREdit must rewrite stored state, and
// fields left empty must be preserved (not blanked).
func TestFakeClient_EditBaseAndBody(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	pr, _ := c.PRCreate(ctx, gh.PRCreateOpts{Title: "t", Base: "pr01", Head: "pr02", Body: "orig"})

	if err := c.PREdit(ctx, pr.Number, gh.PREditOpts{Base: "test_gss"}); err != nil {
		t.Fatalf("PREdit base: %v", err)
	}
	got, _ := c.PRView(ctx, pr.Number)
	if got.Base != "test_gss" {
		t.Errorf("Base = %q; want test_gss", got.Base)
	}
	if got.Body != "orig" {
		t.Errorf("Body = %q; want preserved 'orig' (base-only edit)", got.Body)
	}

	if err := c.PREdit(ctx, pr.Number, gh.PREditOpts{Body: "rewritten"}); err != nil {
		t.Fatalf("PREdit body: %v", err)
	}
	got, _ = c.PRView(ctx, pr.Number)
	if got.Body != "rewritten" || got.Base != "test_gss" {
		t.Errorf("after body edit: %+v; want Body=rewritten Base=test_gss", got)
	}
}

// TestFakeClient_ListFiltersByState — PRList honours the State filter and
// returns results deterministically ordered by number.
func TestFakeClient_ListFiltersByState(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	a, _ := c.PRCreate(ctx, gh.PRCreateOpts{Title: "a", Base: "main", Head: "fa"})
	b, _ := c.PRCreate(ctx, gh.PRCreateOpts{Title: "b", Base: "main", Head: "fb"})
	// Close b by seeding a state change through SeedPR.
	bb, _ := c.PRView(ctx, b.Number)
	bb.State = "MERGED"
	c.SeedPR(bb)

	open, err := c.PRList(ctx, gh.PRFilter{State: "open"})
	if err != nil {
		t.Fatalf("PRList open: %v", err)
	}
	if len(open) != 1 || open[0].Number != a.Number {
		t.Errorf("open list = %+v; want only #%d", open, a.Number)
	}

	all, _ := c.PRList(ctx, gh.PRFilter{State: "all"})
	if len(all) != 2 || all[0].Number > all[1].Number {
		t.Errorf("all list = %+v; want 2 entries sorted ascending", all)
	}
}

// TestFakeClient_PerVerbScripting — an error scripted on PRView must NOT be
// consumed by PRCreate, and only fires once (FIFO within the verb). This is
// the behaviour that distinguishes per-verb scripting from a global FIFO.
func TestFakeClient_PerVerbScripting(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	boom := stderrors.New("boom")
	c.ScriptError(ghfake.VerbPRView, boom)

	// PRCreate must be unaffected by the PRView script.
	pr, err := c.PRCreate(ctx, gh.PRCreateOpts{Title: "t", Base: "main", Head: "f"})
	if err != nil {
		t.Fatalf("PRCreate must ignore PRView script: %v", err)
	}

	// First PRView pops the scripted error.
	if _, err := c.PRView(ctx, pr.Number); !stderrors.Is(err, boom) {
		t.Errorf("first PRView err = %v; want boom", err)
	}
	// Second PRView returns to normal stateful behaviour.
	if _, err := c.PRView(ctx, pr.Number); err != nil {
		t.Errorf("second PRView err = %v; want nil (script drained)", err)
	}
}

// TestFakeClient_RecordsCalls — calls are captured in order with their verb
// and the opts/num passed, so orchestrator tests can assert the gh
// conversation.
func TestFakeClient_RecordsCalls(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	pr, _ := c.PRCreate(ctx, gh.PRCreateOpts{Title: "t", Base: "main", Head: "f"})
	_ = c.PRReady(ctx, pr.Number)

	calls := c.Calls()
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d; want 2", len(calls))
	}
	if calls[0].Verb != ghfake.VerbPRCreate || calls[0].CreateOpts.Title != "t" {
		t.Errorf("calls[0] = %+v; want PRCreate{Title:t}", calls[0])
	}
	if calls[1].Verb != ghfake.VerbPRReady || calls[1].Num != pr.Number {
		t.Errorf("calls[1] = %+v; want PRReady{Num:%d}", calls[1], pr.Number)
	}
}

// TestFakeClient_EmptyInputContract — the empty/invalid-input contract
// (carry-forward note #5), pinned across the mutating verbs.
func TestFakeClient_EmptyInputContract(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	if _, err := c.PRCreate(ctx, gh.PRCreateOpts{Base: "main", Head: "f"}); err == nil {
		t.Error("PRCreate empty Title: want error")
	}
	if _, err := c.PRView(ctx, 0); err == nil {
		t.Error("PRView(0): want error")
	}
	if err := c.PREdit(ctx, 0, gh.PREditOpts{Base: "main"}); err == nil {
		t.Error("PREdit(0): want error")
	}
	if err := c.PRReady(ctx, 0); err == nil {
		t.Error("PRReady(0): want error")
	}
}

// TestFakeClient_ViewNotFound — viewing an unknown PR is a deterministic
// error, not a zero-value PR.
func TestFakeClient_ViewNotFound(t *testing.T) {
	c := ghfake.NewClient()
	if _, err := c.PRView(context.Background(), 999); err == nil {
		t.Error("PRView(unknown): want error")
	}
}

// TestFakeClient_AuthAndRepo — AuthStatus / RepoView return injected values.
func TestFakeClient_AuthAndRepo(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()

	if err := c.AuthStatus(ctx); err != nil {
		t.Errorf("default AuthStatus = %v; want nil (authed)", err)
	}
	authErr := stderrors.New("logged out")
	c.SetAuthErr(authErr)
	if err := c.AuthStatus(ctx); !stderrors.Is(err, authErr) {
		t.Errorf("AuthStatus = %v; want injected authErr", err)
	}

	c.SetRepo(gh.RepoInfo{Owner: "o", Name: "r", NameWithOwner: "o/r", DefaultBranch: "main"})
	info, err := c.RepoView(ctx)
	if err != nil {
		t.Fatalf("RepoView: %v", err)
	}
	if info.DefaultBranch != "main" || info.NameWithOwner != "o/r" {
		t.Errorf("RepoView = %+v", info)
	}
}

// TestFakeClient_SeedFromJSON — the fake is scriptable from the same
// testdata/gh_responses/*.json fixtures the parsers consume (plan PR-03
// acceptance).
func TestFakeClient_SeedFromJSON(t *testing.T) {
	c := ghfake.NewClient()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "gh_responses", "pr_view_ready.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := c.SeedFromJSON(data); err != nil {
		t.Fatalf("SeedFromJSON: %v", err)
	}
	pr, err := c.PRView(context.Background(), 24)
	if err != nil {
		t.Fatalf("PRView seeded: %v", err)
	}
	if pr.Head != "pr02-internal-git-runner" || pr.IsDraft {
		t.Errorf("seeded PR wrong: %+v", pr)
	}
}

// TestFakeClient_ConcurrentSafe — concurrent PRCreate calls produce unique
// numbers and no data race (verified under `go test -race`).
func TestFakeClient_ConcurrentSafe(t *testing.T) {
	c := ghfake.NewClient()
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	nums := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			pr, err := c.PRCreate(context.Background(), gh.PRCreateOpts{Title: "t", Base: "main", Head: "f"})
			if err == nil {
				nums <- pr.Number
			}
		}()
	}
	wg.Wait()
	close(nums)

	seen := map[int]bool{}
	for num := range nums {
		if seen[num] {
			t.Errorf("duplicate PR number %d minted under concurrency", num)
		}
		seen[num] = true
	}
	if len(seen) != n {
		t.Errorf("minted %d unique numbers; want %d", len(seen), n)
	}
	if c.PRCount() != n {
		t.Errorf("PRCount = %d; want %d", c.PRCount(), n)
	}
}

// TestFakeClient_Reset — Reset returns the fake to a freshly-constructed
// state (no PRs, no calls, no scripts).
func TestFakeClient_Reset(t *testing.T) {
	c := ghfake.NewClient()
	ctx := context.Background()
	_, _ = c.PRCreate(ctx, gh.PRCreateOpts{Title: "t", Base: "main", Head: "f"})
	c.ScriptError(ghfake.VerbPRView, stderrors.New("x"))

	c.Reset()

	if c.PRCount() != 0 {
		t.Errorf("after Reset PRCount = %d; want 0", c.PRCount())
	}
	if len(c.Calls()) != 0 {
		t.Errorf("after Reset Calls = %d; want 0", len(c.Calls()))
	}
	// A fresh create should mint #1 again and PRView should not error.
	pr, _ := c.PRCreate(ctx, gh.PRCreateOpts{Title: "t", Base: "main", Head: "f"})
	if pr.Number != 1 {
		t.Errorf("post-Reset first PR number = %d; want 1", pr.Number)
	}
	if _, err := c.PRView(ctx, pr.Number); err != nil {
		t.Errorf("post-Reset PRView err = %v; want nil (script cleared)", err)
	}
}
