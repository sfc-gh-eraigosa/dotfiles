// Package classic_test verifies the pr orchestrator per
// sdk/gss/docs/plan.md PR-24: timestamped feature-branch generation on the
// default branch, PR open/create via gh.Client, and approval-token
// consumption. Reuses fakeApprover from push_test.go (same test package).
package classic_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/classic"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func clock202605211234() fixedClock {
	return fixedClock{t: time.Date(2026, 5, 21, 12, 34, 56, 0, time.UTC)}
}

func newPRer(gitr *gitfake.Runner, ghc *ghfake.Client, out *bytes.Buffer) (*classic.PRer, *fakeApprover) {
	ap := &fakeApprover{}
	return &classic.PRer{Git: gitr, GH: ghc, Approval: ap, Clock: clock202605211234(), Out: out}, ap
}

func TestPR_DefaultBranchCreatesTimestampedFeature(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("main\n")}, // rev-parse --abbrev-ref HEAD
		{},                         // status --porcelain (clean)
		{Stdout: []byte("1\n")},    // rev-list --count origin/main..HEAD (ahead)
		{},                         // checkout -b feature/gss-<ts>
		{},                         // push -u
	}}
	ghc := ghfake.NewClient() // no PRs → create
	var out bytes.Buffer
	p, ap := newPRer(gitr, ghc, &out)

	if err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("PR: %v", err)
	}
	if ap.calls != 1 {
		t.Errorf("approval calls = %d; want 1 (token consumed)", ap.calls)
	}
	const wantBranch = "feature/gss-20260521-123456"
	// checkout -b call (4th, after the two preflight reads) must name the
	// timestamped branch.
	if !argsHas(gitr.Calls[3].Args, "checkout") || !argsHas(gitr.Calls[3].Args, "-b") || !argsHas(gitr.Calls[3].Args, wantBranch) {
		t.Errorf("call[3] = %+v; want `checkout -b %s`", gitr.Calls[3], wantBranch)
	}
	// PRCreate must target that branch.
	created := lastPRCreate(ghc)
	if created == nil || created.CreateOpts.Head != wantBranch || created.CreateOpts.Base != "main" {
		t.Errorf("PRCreate = %+v; want Head=%s Base=main", created, wantBranch)
	}
	if !strings.Contains(out.String(), "Pull Request created") {
		t.Errorf("output missing 'Pull Request created':\n%s", out.String())
	}
}

func TestPR_FeatureBranchUsesIt(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("feature/x\n")}, // rev-parse
		{},                              // push
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _ := newPRer(gitr, ghc, &out)

	if err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("PR: %v", err)
	}
	// No checkout -b on an existing feature branch (only rev-parse + push).
	for _, c := range gitr.Calls {
		if argsHas(c.Args, "checkout") {
			t.Errorf("unexpected checkout on existing feature branch: %+v", c)
		}
	}
	if created := lastPRCreate(ghc); created == nil || created.CreateOpts.Head != "feature/x" {
		t.Errorf("PRCreate Head = %+v; want feature/x", created)
	}
}

func TestPR_ExistingPRSurfaced(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("feature/x\n")},
		{},
	}}
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 7, Head: "feature/x", State: "OPEN", URL: "https://github.com/o/r/pull/7"})
	var out bytes.Buffer
	p, _ := newPRer(gitr, ghc, &out)

	if err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("PR: %v", err)
	}
	if !strings.Contains(out.String(), "Pull Request already exists: https://github.com/o/r/pull/7") {
		t.Errorf("expected existing-PR surfaced; got:\n%s", out.String())
	}
	if lastPRCreate(ghc) != nil {
		t.Error("existing PR present; must not PRCreate")
	}
}

// TestPR_DefaultBranchDirtyNoCommitsFailsFast pins the fix for the
// "gss pr on a dirty default branch" trap: with no commits ahead of
// origin/<default>, cutting a branch produces a PR GitHub rejects with
// the opaque "No commits between main and <branch>" — after the empty
// branch was already created and pushed. The orchestrator must instead
// fail fast, name the uncommitted changes as the likely cause, and
// leave the repo untouched (no checkout, no push, no PRCreate).
func TestPR_DefaultBranchDirtyNoCommitsFailsFast(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("main\n")},              // rev-parse --abbrev-ref HEAD
		{Stdout: []byte(" M opt/bin/docker\n")}, // status --porcelain (dirty)
		{Stdout: []byte("0\n")},                 // rev-list --count (not ahead)
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _ := newPRer(gitr, ghc, &out)

	err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"})
	if err == nil {
		t.Fatal("PR succeeded; want fail-fast error for dirty tree with no commits ahead")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("err = %v; want mention of uncommitted changes", err)
	}
	for _, c := range gitr.Calls {
		if argsHas(c.Args, "checkout") || argsHas(c.Args, "push") {
			t.Errorf("preflight failure must not checkout/push; got %+v", c)
		}
	}
	if lastPRCreate(ghc) != nil {
		t.Error("preflight failure must not PRCreate")
	}
}

// TestPR_DefaultBranchCleanNoCommitsFailsFast: same guard, clean tree —
// there is simply nothing to PR.
func TestPR_DefaultBranchCleanNoCommitsFailsFast(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("main\n")}, // rev-parse
		{},                         // status --porcelain (clean)
		{Stdout: []byte("0\n")},    // rev-list --count (not ahead)
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _ := newPRer(gitr, ghc, &out)

	err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"})
	if err == nil {
		t.Fatal("PR succeeded; want fail-fast error when nothing is ahead of origin")
	}
	if !strings.Contains(err.Error(), "no commits ahead") {
		t.Errorf("err = %v; want 'no commits ahead'", err)
	}
	if lastPRCreate(ghc) != nil {
		t.Error("preflight failure must not PRCreate")
	}
}

// TestPR_DefaultBranchNoOriginRefSkipsAheadCheck: when origin/<default>
// can't be resolved (fresh repo, brand-new remote), the ahead-count is
// meaningless — the preflight steps aside and the flow proceeds.
func TestPR_DefaultBranchNoOriginRefSkipsAheadCheck(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("main\n")}, // rev-parse
		{},                         // status --porcelain
		{Err: stderrors.New("unknown revision origin/main")}, // rev-list fails
		{}, // checkout -b
		{}, // push -u
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _ := newPRer(gitr, ghc, &out)

	if err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("PR: %v (missing origin ref must not block)", err)
	}
	if created := lastPRCreate(ghc); created == nil {
		t.Error("want PRCreate when origin/<default> is unresolvable")
	}
}

func TestPR_ApprovalBlocks(t *testing.T) {
	gitr := &gitfake.Runner{}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, ap := newPRer(gitr, ghc, &out)
	ap.err = errors.ErrApprovalTokenMissing

	if err := p.PR(context.Background(), classic.PROpts{RepoPath: "/r", DefaultBranch: "main"}); !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Fatalf("PR err = %v; want ErrApprovalTokenMissing", err)
	}
	if gitr.CallCount() != 0 {
		t.Errorf("approval failure must abort before git; calls=%d", gitr.CallCount())
	}
}

func argsHas(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func lastPRCreate(c *ghfake.Client) *ghfake.Call {
	calls := c.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Verb == ghfake.VerbPRCreate {
			return &calls[i]
		}
	}
	return nil
}
