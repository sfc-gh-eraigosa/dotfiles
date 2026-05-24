// Package classic_test verifies the push orchestrator per
// src/gss/docs/plan.md PR-22: the approval → backup → sync → push → auto-PR
// flow, driven by trivial service fakes + the fake git.Runner / gh.Client.
package classic_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/classic"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/sync"
)

type fakeApprover struct {
	err   error
	calls int
}

func (f *fakeApprover) Verify(_ context.Context, _ string, _ bool) error { f.calls++; return f.err }

type fakeBackup struct {
	name  string
	err   error
	calls int
}

func (f *fakeBackup) Create(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.name, f.err
}

type fakeSync struct {
	err   error
	calls int
}

func (f *fakeSync) Sync(_ context.Context, _ string) (sync.Result, error) {
	f.calls++
	return sync.Result{Branch: "x"}, f.err
}

// newPusher wires a Pusher with the given git/gh fakes and fresh service
// fakes (returned for assertions).
func newPusher(gitr *gitfake.Runner, ghc *ghfake.Client, out *bytes.Buffer) (*classic.Pusher, *fakeApprover, *fakeBackup, *fakeSync) {
	ap := &fakeApprover{}
	bk := &fakeBackup{name: "backup/gss-20260521-000000"}
	sy := &fakeSync{}
	p := &classic.Pusher{Git: gitr, GH: ghc, Approval: ap, Backup: bk, Sync: sy, Out: out}
	return p, ap, bk, sy
}

func gitCallHasArg(calls []gitfake.CallRecord, idx int, want string) bool {
	if idx >= len(calls) {
		return false
	}
	for _, a := range calls[idx].Args {
		if a == want {
			return true
		}
	}
	return false
}

func TestPush_FeatureBranchCreatesPR(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("feature/x\n")}, // rev-parse --abbrev-ref HEAD
		{},                              // push
	}}
	ghc := ghfake.NewClient() // no PRs → PRList empty → PRCreate
	var out bytes.Buffer
	p, ap, bk, sy := newPusher(gitr, ghc, &out)

	err := p.Push(context.Background(), classic.PushOpts{RepoPath: "/r", DefaultBranch: "main", PRTitle: "My change"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if ap.calls != 1 || bk.calls != 1 || sy.calls != 1 {
		t.Errorf("service calls: approver=%d backup=%d sync=%d; want 1/1/1", ap.calls, bk.calls, sy.calls)
	}
	for _, want := range []string{"Step 1", "Step 2", "Step 3", "Successfully pushed", "Pull Request created"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	// push (2nd git call) must carry -u on a feature branch.
	if !gitCallHasArg(gitr.Calls, 1, "push") || !gitCallHasArg(gitr.Calls, 1, "-u") {
		t.Errorf("push call = %+v; want `push -u`", gitr.Calls[1])
	}
	// A PR was created for the feature branch.
	var created *ghfake.Call
	for i := range ghc.Calls() {
		if c := ghc.Calls()[i]; c.Verb == ghfake.VerbPRCreate {
			created = &c
		}
	}
	if created == nil || created.CreateOpts.Head != "feature/x" || created.CreateOpts.Base != "main" {
		t.Errorf("PRCreate = %+v; want Head=feature/x Base=main", created)
	}
}

func TestPush_DefaultBranchNoPR(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("main\n")},
		{},
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _, _, _ := newPusher(gitr, ghc, &out)

	if err := p.Push(context.Background(), classic.PushOpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	// No -u on the default branch.
	if gitCallHasArg(gitr.Calls, 1, "-u") {
		t.Errorf("default-branch push must not set -u: %+v", gitr.Calls[1])
	}
	// No PR work on the default branch.
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRCreate || c.Verb == ghfake.VerbPRList {
			t.Errorf("default branch should not touch PRs; saw %s", c.Verb)
		}
	}
}

func TestPush_ApprovalBlocks(t *testing.T) {
	gitr := &gitfake.Runner{}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, ap, bk, sy := newPusher(gitr, ghc, &out)
	ap.err = errors.ErrApprovalTokenMissing

	err := p.Push(context.Background(), classic.PushOpts{RepoPath: "/r", DefaultBranch: "main"})
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Fatalf("Push err = %v; want ErrApprovalTokenMissing", err)
	}
	if bk.calls != 0 || sy.calls != 0 {
		t.Errorf("approval failure must abort before backup/sync; backup=%d sync=%d", bk.calls, sy.calls)
	}
	if gitr.CallCount() != 0 {
		t.Errorf("approval failure must not touch git; calls=%d", gitr.CallCount())
	}
}

func TestPush_ExistingPRSurfaced(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("feature/x\n")},
		{},
	}}
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 5, Head: "feature/x", State: "OPEN", URL: "https://github.com/o/r/pull/5"})
	var out bytes.Buffer
	p, _, _, _ := newPusher(gitr, ghc, &out)

	if err := p.Push(context.Background(), classic.PushOpts{RepoPath: "/r", DefaultBranch: "main"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !strings.Contains(out.String(), "Pull Request: https://github.com/o/r/pull/5") {
		t.Errorf("expected existing PR surfaced; got:\n%s", out.String())
	}
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRCreate {
			t.Error("existing PR present; must not PRCreate")
		}
	}
}

func TestPush_PushError(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		{Stdout: []byte("feature/x\n")},
		{Err: stderrors.New("rejected: non-fast-forward")},
	}}
	ghc := ghfake.NewClient()
	var out bytes.Buffer
	p, _, _, _ := newPusher(gitr, ghc, &out)

	if err := p.Push(context.Background(), classic.PushOpts{RepoPath: "/r", DefaultBranch: "main"}); err == nil {
		t.Error("Push with failing git push: err = nil; want error")
	}
}
