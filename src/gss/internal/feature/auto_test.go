package feature_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
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
	svc := &feature.Service{Store: store, Git: gitr, GH: ghc, Clock: fixedClock{t: timeFixed()}}
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
	data, _ := os.ReadFile(filepath.Join(wt, "WORKER.md"))
	if !strings.Contains(string(data), "detached") {
		t.Errorf("WORKER.md missing diagnostic:\n%s", data)
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
	data, _ := os.ReadFile(filepath.Join(wt, "WORKER.md"))
	if !strings.Contains(string(data), "rebase conflict") {
		t.Errorf("WORKER.md missing conflict diagnostic:\n%s", data)
	}
}

func TestAuto_ReadyPRBodyOnlyNoNewCommits(t *testing.T) {
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 9, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: false, URL: "https://github.com/o/r/pull/9"})
	// dirty → commit, then ready-PR → body edit only (no fetch/rebase/push).
	svc, gitr, _ := autoService(t, "https://github.com/o/r/pull/9",
		[]gitfake.Response{resp("feature/auth/erai/api"), resp(" M a.go\n"), resp("aaa"), resp("bbb"), resp(""), resp("")}, ghc)

	if _, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("AutoCheckpoint: %v", err)
	}
	// A ready PR must NOT get new pushed commits → Checkpoint never runs → no fetch.
	if gitCallsHave(gitr, "fetch") {
		t.Error("ready PR must not push new commits (no fetch/rebase via Checkpoint)")
	}
	edited := false
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPREdit && c.Num == 9 {
			edited = true
		}
	}
	if !edited {
		t.Error("ready PR should get a body refresh (PREdit)")
	}
}
