package feature_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

func timeFixed() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }

// autoService seeds one worker (worktree is a real temp dir so WORKER.md
// writes succeed) and wires the git/gh fakes.
func autoService(t *testing.T, prURL string, script []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *gitfake.Runner, string) {
	t.Helper()
	root := t.TempDir()
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: wt, BaseBranch: "main", Description: "endpoints", PRURL: prURL,
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitr := &gitfake.Runner{Script: script}
	// Approval defaults to a succeeding fake: AutoCheckpoint's happy-path
	// tests exercise the push path (via the delegate-to-Checkpoint call),
	// which is now gated (dotfiles#331) — see TestAutoCheckpoint_* below for
	// the gate's own negative-path tests, which build the Service directly.
	svc := &feature.Service{Store: store, Git: gitr, GH: ghc, Clock: fixedClock{t: timeFixed()}, Approval: &fakeApprover{}}
	return svc, gitr, wt
}

func resp(s string) gitfake.Response { return gitfake.Response{Stdout: []byte(s)} }
func errResp() gitfake.Response      { return gitfake.Response{Err: stderrors.New("git failed")} }
func gitCallsHave(gitr *gitfake.Runner, want string) bool {
	for _, c := range gitr.Calls {
		if argsHasFC(c.Args, want) {
			return true
		}
	}
	return false
}

func TestAuto_DryRunPlansWithoutExecuting(t *testing.T) {
	svc, gitr, _ := autoService(t, "",
		[]gitfake.Response{resp("feature/auth/erai/api"), resp(" M a.go\n"), resp("aaa"), resp("bbb")}, ghfake.NewClient())
	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api", DryRun: true})
	if err != nil {
		t.Fatalf("AutoCheckpoint dry-run: %v", err)
	}
	if len(res.Planned) == 0 {
		t.Error("dry-run should produce a plan")
	}
	if gitCallsHave(gitr, "add") || gitCallsHave(gitr, "commit") || gitCallsHave(gitr, "fetch") {
		t.Errorf("dry-run must not execute mutations; calls=%+v", gitr.Calls)
	}
}

func TestAuto_NoOpWhenCleanAndSynced(t *testing.T) {
	svc, gitr, _ := autoService(t, "",
		[]gitfake.Response{resp("feature/auth/erai/api"), resp(""), resp("aaa"), resp("aaa")}, ghfake.NewClient())
	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("AutoCheckpoint: %v", err)
	}
	if !res.NoOp {
		t.Errorf("clean + synced should be a no-op; got %+v", res)
	}
	if gitCallsHave(gitr, "commit") || gitCallsHave(gitr, "fetch") {
		t.Errorf("no-op must not mutate; calls=%+v", gitr.Calls)
	}
}

func TestAuto_DetachedHEADSkipsWithDiagnostic(t *testing.T) {
	svc, _, wt := autoService(t, "", []gitfake.Response{resp("HEAD")}, ghfake.NewClient())
	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err == nil {
		t.Fatal("detached HEAD: want non-zero (error) result")
	}
	if !strings.Contains(res.Skipped, "detached") {
		t.Errorf("Skipped = %q; want detached-HEAD reason", res.Skipped)
	}
	// The diagnostic lands at the meta path OUTSIDE the worktree (issue #132),
	// even though appendAutoLog is the first writer here (empty worktree).
	if _, err := os.Stat(filepath.Join(wt, "WORKER.md")); !os.IsNotExist(err) {
		t.Errorf("auto-log must not write WORKER.md into the worktree root (#132); err=%v", err)
	}
	data, _ := os.ReadFile(feature.WorkerMetaPath(wt))
	if !strings.Contains(string(data), "detached") {
		t.Errorf("WORKER.md (meta path) missing diagnostic:\n%s", data)
	}
}

func TestAuto_DirtyCommitsTrackedOnly(t *testing.T) {
	// abbrev, status(tracked+untracked), HEAD, origin(err=not pushed),
	// add, commit, then Checkpoint: fetch, rebase.
	svc, gitr, _ := autoService(t, "",
		[]gitfake.Response{
			resp("feature/auth/erai/api"), resp(" M a.go\n?? new.txt\n"), resp("aaa"), errResp(),
			resp(""), resp(""), // add, commit
			resp(""), resp(""), // checkpoint: fetch, rebase
		}, ghfake.NewClient())

	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("AutoCheckpoint: %v", err)
	}
	if !res.Committed {
		t.Error("dirty worktree should produce a WIP commit")
	}
	// Find the `add` call: must stage a.go via explicit `--`, never -A, never the untracked file.
	var addCall *gitfake.CallRecord
	for i := range gitr.Calls {
		if argsHasFC(gitr.Calls[i].Args, "add") {
			addCall = &gitr.Calls[i]
		}
	}
	if addCall == nil {
		t.Fatal("no git add call")
	}
	if !argsHasFC(addCall.Args, "a.go") || !argsHasFC(addCall.Args, "--") {
		t.Errorf("add must stage tracked a.go via --: %+v", addCall.Args)
	}
	if argsHasFC(addCall.Args, "-A") || argsHasFC(addCall.Args, "new.txt") {
		t.Errorf("add must NOT use -A or stage untracked files: %+v", addCall.Args)
	}
}

func TestAuto_RebaseConflictSkips(t *testing.T) {
	// clean (no commit) but needs push; Checkpoint rebase conflicts.
	svc, _, wt := autoService(t, "",
		[]gitfake.Response{
			resp("feature/auth/erai/api"), resp(""), resp("aaa"), resp("bbb"),
			resp(""), errResp(), resp(""), // checkpoint: fetch, rebase(fail), rebase --abort
		}, ghfake.NewClient())

	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err == nil {
		t.Fatal("rebase conflict: want non-zero result")
	}
	if !strings.Contains(res.Skipped, "rebase conflict") {
		t.Errorf("Skipped = %q; want rebase-conflict reason", res.Skipped)
	}
	data, _ := os.ReadFile(feature.WorkerMetaPath(wt))
	if !strings.Contains(string(data), "rebase conflict") {
		t.Errorf("WORKER.md (meta path) missing conflict diagnostic:\n%s", data)
	}
}

func TestAuto_ReadyPRWithPendingWorkSkipsLoudly(t *testing.T) {
	// The silent-success shape this guards against (observed live on PR #153):
	// a ready-for-review PR + local commits ahead of origin → the pre-fix code
	// refreshed the body, pushed NOTHING, printed "updated the draft PR", and
	// exited 0. A ready PR with pending work is a prompt-condition: body
	// refresh is fine, but the result must be a SKIP (non-zero) with a
	// diagnostic, never success.
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 9, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: false, URL: "https://github.com/o/r/pull/9"})
	// dirty → WIP commit, then ready-PR → body edit + loud skip (no fetch/rebase/push).
	svc, gitr, wt := autoService(t, "https://github.com/o/r/pull/9",
		[]gitfake.Response{resp("feature/auth/erai/api"), resp(" M a.go\n"), resp("aaa"), resp("bbb"), resp(""), resp("")}, ghc)

	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err == nil {
		t.Fatal("ready PR with pending work: want non-zero (skip) result, got success")
	}
	if !strings.Contains(res.Skipped, "ready-for-review") || !strings.Contains(res.Skipped, "NOT push") {
		t.Errorf("Skipped = %q; want ready-for-review draft-only reason", res.Skipped)
	}
	if !res.Committed {
		t.Error("the WIP commit made before the ready-PR check should be reported")
	}
	// A ready PR must NOT get new pushed commits → Checkpoint never runs → no fetch/push.
	if gitCallsHave(gitr, "fetch") || gitCallsHave(gitr, "push") {
		t.Error("ready PR must not push new commits (no fetch/rebase/push via Checkpoint)")
	}
	edited := false
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPREdit && c.Num == 9 {
			edited = true
		}
	}
	if !edited {
		t.Error("ready PR should still get a body refresh (PREdit)")
	}
	// The skip diagnostic must land in the worker's meta WORKER.md.
	data, _ := os.ReadFile(feature.WorkerMetaPath(wt))
	if !strings.Contains(string(data), "ready-for-review") {
		t.Errorf("WORKER.md (meta path) missing ready-PR diagnostic:\n%s", data)
	}
}

// TestAutoCheckpoint_PropagatesApprovalGate pins the dotfiles#331 fix on the
// path a process hook actually uses: --auto delegates its push to
// Checkpoint (auto.go:139), so gating Checkpoint must also gate the auto
// variant. A missing/stale token must fail loudly (non-zero) rather than
// pushing silently — before the fix, checkpoint pushed and reported success
// with a deliberately stale approval.token in place.
func TestAutoCheckpoint_PropagatesApprovalGate(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: wt, BaseBranch: "main", Description: "endpoints",
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// branch, porcelain (clean), local HEAD, remote HEAD (diverged) → needPush
	// without needCommit, so AutoCheckpoint delegates straight to Checkpoint.
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("feature/auth/erai/api"), resp(""), resp("aaa"), resp("bbb"),
	}}
	ap := &fakeApprover{err: errors.ErrApprovalTokenMissing}
	svc := &feature.Service{Store: store, Git: gitr, GH: ghfake.NewClient(), Clock: fixedClock{t: timeFixed()}, Approval: ap}

	_, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Fatalf("err = %v; want ErrApprovalTokenMissing", err)
	}
	if ap.calls == 0 {
		t.Error("approval verifier was never consulted")
	}
	if gitCallsHave(gitr, "push") {
		t.Error("auto-checkpoint must not push when the approval gate refuses")
	}
}

// TestAutoCheckpoint_ApprovalGateBlocksLocalCommit pins a finding from the
// PR #333 review: the previous fix only proved the gate ran before PUSH.
// AutoCheckpoint's own WIP `git add`/`git commit` (for needCommit) runs
// BEFORE it delegates to Checkpoint, so with the gate placed only inside
// Checkpoint, a missing/stale token still let a local commit land — this
// contradicts the stated invariant ("a refusal touches neither git nor
// gh") and could surprise a user who deliberately withheld approval. The
// gate must be checked before ANY git mutation, including the local commit,
// and checked exactly ONCE along this path (the token is single-use —
// consumed on a successful Verify — so a second Verify inside a delegated
// Checkpoint call would spuriously fail against an already-consumed token).
func TestAutoCheckpoint_ApprovalGateBlocksLocalCommit(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: wt, BaseBranch: "main", Description: "endpoints",
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// branch, porcelain (one tracked change) → needCommit=true. If the gate
	// refuses before any mutation, git never even reaches "add"/"commit".
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("feature/auth/erai/api"), resp(" M a.go\n"), resp("aaa"), resp("bbb"),
	}}
	ap := &fakeApprover{err: errors.ErrApprovalTokenMissing}
	svc := &feature.Service{Store: store, Git: gitr, GH: ghfake.NewClient(), Clock: fixedClock{t: timeFixed()}, Approval: ap}

	_, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Fatalf("err = %v; want ErrApprovalTokenMissing", err)
	}
	if gitCallsHave(gitr, "add") || gitCallsHave(gitr, "commit") {
		t.Errorf("auto-checkpoint must not commit locally when the approval gate refuses; calls=%+v", gitr.Calls)
	}
	if ap.calls != 1 {
		t.Errorf("approval verifier consulted %d times; want exactly 1", ap.calls)
	}
}

// onceApprover simulates the real approval.Verifier's single-use token: it
// succeeds on its first call and fails every call after, since a real token
// is consumed (deleted) on a successful Verify.
type onceApprover struct{ calls int }

func (a *onceApprover) Verify(context.Context, string, bool) error {
	a.calls++
	if a.calls > 1 {
		return errors.ErrApprovalTokenMissing
	}
	return nil
}

// TestAutoCheckpoint_VerifiesTokenExactlyOnceOnHappyPath pins the other
// half of the ordering fix: moving the gate earlier in AutoCheckpoint must
// NOT introduce a second Verify call when it later delegates to Checkpoint
// — with a real (single-use) token verifier, a second Verify on the same
// call chain would spuriously fail even though the first one succeeded,
// breaking every successful auto-checkpoint that needs to push.
func TestAutoCheckpoint_VerifiesTokenExactlyOnceOnHappyPath(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: wt, BaseBranch: "main", Description: "endpoints",
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// branch, porcelain (dirty) -> needCommit; local, remote (diverged) ->
	// needPush; then add, commit; then Checkpoint's own fetch, rebase, push,
	// pr create.
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("feature/auth/erai/api"), resp(" M a.go\n"), resp("aaa"), resp("bbb"),
		{}, {}, // add, commit
		{}, {}, {}, // fetch, rebase, push (create path)
	}}
	ap := &onceApprover{}
	ghc := ghfake.NewClient()
	svc := &feature.Service{Store: store, Git: gitr, GH: ghc, Clock: fixedClock{t: timeFixed()}, Approval: ap}

	res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("AutoCheckpoint: %v", err)
	}
	if !res.Committed {
		t.Error("expected the WIP commit to be reported")
	}
	if ap.calls != 1 {
		t.Errorf("approval verifier consulted %d times; want exactly 1 (a second Verify would fail against a real single-use token)", ap.calls)
	}
}
