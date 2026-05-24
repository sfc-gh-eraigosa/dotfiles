// Package classic_test verifies the pr orchestrator per
// src/gss/docs/plan.md PR-24: timestamped feature-branch generation on the
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

	"github.com/wenlock/dotfiles/gss/internal/classic"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
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
	// checkout -b call (2nd) must name the timestamped branch.
	if !argsHas(gitr.Calls[1].Args, "checkout") || !argsHas(gitr.Calls[1].Args, "-b") || !argsHas(gitr.Calls[1].Args, wantBranch) {
		t.Errorf("call[1] = %+v; want `checkout -b %s`", gitr.Calls[1], wantBranch)
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
