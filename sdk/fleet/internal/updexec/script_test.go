package updexec

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func repo(name string, branches []string) updplan.Repo {
	return updplan.Repo{
		Name:     name,
		Path:     "~/git/" + name,
		Branches: branches,
		Local:    updplan.LocalSkip,
		Restore:  true,
	}
}

func mustSync(t *testing.T, r updplan.Repo, local updplan.Local, reset bool) string {
	t.Helper()
	s, err := SyncScript(r, local, reset)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestSyncScriptSingleBranchMatchesTodaysForm pins the single-branch BODY to
// exactly today's text (cmd/update.go's updateScript, minus the trailing
// "&& ./install.sh" — that is now a separate run step).
func TestSyncScriptSingleBranchMatchesTodaysForm(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main"}), updplan.LocalSkip, false)
	want := "git fetch origin main && git checkout main && git merge --ff-only FETCH_HEAD"
	if !strings.Contains(s, want) {
		t.Fatalf("single-branch BODY not byte-compatible:\ngot:  %s\nwant substring: %s", s, want)
	}
}

// TestUpdateMakesExactlyOneNetworkCall is moved verbatim (four assertions)
// from cmd/update_test.go, retargeted at SyncScript.
func TestUpdateMakesExactlyOneNetworkCall(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main"}), updplan.LocalSkip, false)
	if n := strings.Count(s, "git fetch"); n != 1 {
		t.Fatalf("expected exactly one fetch, got %d:\n%s", n, s)
	}
	if strings.Contains(s, "git pull") {
		t.Fatalf("git pull re-contacts the remote; merge the fetched ref instead:\n%s", s)
	}
	if !strings.Contains(s, "merge --ff-only FETCH_HEAD") {
		t.Fatalf("the fast-forward must be local:\n%s", s)
	}
	if !strings.Contains(s, "--ff-only") {
		t.Fatalf("must stay fast-forward-only:\n%s", s)
	}
}

func networkCallCount(s string) int {
	return strings.Count(s, "git fetch") + strings.Count(s, "git clone")
}

// TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall covers single,
// multi, and default forms (the clone form is covered once CloneScript
// lands in task 7).
func TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall(t *testing.T) {
	cases := []struct {
		name string
		s    string
	}{
		{"single", mustSync(t, repo("dotfiles", []string{"main"}), updplan.LocalSkip, false)},
		{"multi", mustSync(t, repo("dotfiles", []string{"main", "staging"}), updplan.LocalSkip, false)},
		{"default", mustSync(t, repo("dotfiles", []string{"default"}), updplan.LocalSkip, false)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if n := networkCallCount(c.s); n != 1 {
				t.Fatalf("%s: expected exactly one network call, got %d:\n%s", c.name, n, c.s)
			}
			if strings.Contains(c.s, "git pull") {
				t.Fatalf("%s: must never git pull:\n%s", c.name, c.s)
			}
			if n := strings.Count(c.s, "ls-remote"); n > 1 {
				t.Fatalf("%s: ls-remote must be used at most once:\n%s", c.name, c.s)
			}
			if strings.Contains(c.s, "ls-remote") {
				before, _, _ := strings.Cut(c.s, "ls-remote")
				if !strings.Contains(before, `symbolic-ref -q --short refs/remotes/origin/HEAD`) {
					t.Fatalf("%s: ls-remote must only run after the local symbolic-ref probe:\n%s", c.name, c.s)
				}
			}
		})
	}

	rClone := repo("dotfiles", []string{"main"})
	rClone.URL = "https://github.com/example/dotfiles.git"
	cs, err := CloneScript(rClone)
	if err != nil {
		t.Fatal(err)
	}
	if n := networkCallCount(cs); n != 1 {
		t.Fatalf("clone: expected exactly one network call, got %d:\n%s", n, cs)
	}
	if strings.Contains(cs, "git fetch") {
		t.Fatalf("clone must never also fetch:\n%s", cs)
	}
}

// --- task 7 -----------------------------------------------------------------

func TestMultiBranchFetchesAllInOneCall(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main", "staging"}), updplan.LocalSkip, false)
	if !strings.Contains(s, "git fetch origin main staging") {
		t.Fatalf("multi-branch must fetch all refs in one call:\n%s", s)
	}
}

func TestExtrasOnlyForceMoveAnAncestor(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"main", "staging"}), updplan.LocalSkip, false)
	if !strings.Contains(s, `git merge-base --is-ancestor "$b" "origin/$b"`) {
		t.Fatalf("extras must gate the branch move on ancestry:\n%s", s)
	}
	if !strings.Contains(s, `git branch -q -f "$b" "origin/$b"`) {
		t.Fatalf("an ancestor branch must be force-moved:\n%s", s)
	}
	if !strings.Contains(s, `echo "fleet: skipped(diverged) $b"`) {
		t.Fatalf("a diverged branch must be reported skipped, not moved:\n%s", s)
	}
	if !strings.Contains(s, `[ "$b" = "$b1" ] && continue`) {
		t.Fatalf("extras must guard against re-processing b1:\n%s", s)
	}
}

func TestDefaultBranchPrefersLocalSymbolicRef(t *testing.T) {
	s := mustSync(t, repo("dotfiles", []string{"default"}), updplan.LocalSkip, false)
	if !strings.Contains(s, `git symbolic-ref -q --short refs/remotes/origin/HEAD`) {
		t.Fatalf("default form must probe the local symbolic-ref first:\n%s", s)
	}
	symIdx := strings.Index(s, "symbolic-ref -q --short refs/remotes/origin/HEAD")
	lsIdx := strings.Index(s, "ls-remote")
	if symIdx < 0 || lsIdx < 0 || symIdx > lsIdx {
		t.Fatalf("local symbolic-ref must be tried before ls-remote:\n%s", s)
	}
}

func TestCloneNeverFetches(t *testing.T) {
	r := repo("dotfiles", []string{"main"})
	r.URL = "https://github.com/example/dotfiles.git"
	s, err := CloneScript(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "git fetch") {
		t.Fatalf("clone must never also fetch:\n%s", s)
	}
	if !strings.Contains(s, "git clone -q --branch main") {
		t.Fatalf("explicit-branch clone must pass --branch:\n%s", s)
	}
}

func TestPrecheckUsesDashEForWorktrees(t *testing.T) {
	s, err := PrecheckScript(repo("dotfiles", []string{"main"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "[ ! -e ") {
		t.Fatalf("precheck must use -e, not -d, so a worktree's .git FILE is detected:\n%s", s)
	}
}

func TestPrecheckReportsStateAndBranchReadOnly(t *testing.T) {
	s, err := PrecheckScript(repo("dotfiles", []string{"main"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"git checkout", "git reset", "git stash", "git branch -f", "git merge", "git fetch", "git clone", "git add", "git commit", "git rm"} {
		if strings.Contains(s, verb) {
			t.Fatalf("precheck must be read-only; found %q:\n%s", verb, s)
		}
	}
	if !strings.Contains(s, `echo "state=$s branch=$b"`) {
		t.Fatalf("precheck must report state and branch:\n%s", s)
	}
}

// TestRescuePreservesUntrackedWork is moved from cmd/update_test.go,
// parameterised on a repo path/name.
func TestRescuePreservesUntrackedWork(t *testing.T) {
	s, err := RescueScript(repo("dotfiles", []string{"main"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "git add -A") {
		t.Fatal("rescue must stage untracked files (git add -A), or they are lost")
	}
	if strings.Contains(s, "stash@{0}") {
		t.Fatal("branching from stash@{0} silently drops untracked files")
	}
	if !strings.Contains(s, "worktree add") {
		t.Fatal("rescued work must be materialised as an inspectable worktree")
	}
	if !strings.Contains(s, "rescue/dotfiles") {
		t.Fatalf("rescue dir must contain the repo name:\n%s", s)
	}
}

func TestResetScriptUnchanged(t *testing.T) {
	s := ResetScript("main")
	want := `ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
		`git checkout -q -b "fleet-reset/$ts" && git add -A && ` +
		`{ git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet pre-reset $ts" || true; } && ` +
		`git checkout -q "main" && git reset --hard FETCH_HEAD`
	if s != want {
		t.Fatalf("ResetScript changed:\ngot:  %s\nwant: %s", s, want)
	}
}

func TestRunScriptIsVerbatimAfterCd(t *testing.T) {
	r := repo("work", []string{"main"})
	r.Path = "~/git/work/scripts"
	st := updplan.Step{ID: "scripts.make", Kind: updplan.KindRun, Run: "make install"}
	s, err := RunScript(st, &r)
	if err != nil {
		t.Fatal(err)
	}
	if s != "cd ~/git/work/scripts && make install" {
		t.Fatalf("run script not verbatim after cd: %q", s)
	}

	s2, err := RunScript(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s2 != "make install" || strings.Contains(s2, "cd ") {
		t.Fatalf("no-repo run step must have no cd: %q", s2)
	}
}

func TestGhAuthNeverCarriesAToken(t *testing.T) {
	check, err := GhAuthCheck("github.com")
	if err != nil {
		t.Fatal(err)
	}
	login, err := GhAuthLogin("github.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{check, login} {
		for _, bad := range []string{"GH_TOKEN", "GITHUB_TOKEN", "--with-token", "token"} {
			if strings.Contains(s, bad) {
				t.Fatalf("gh-auth script must never carry a token (%q): %s", bad, s)
			}
		}
	}
}

func TestGhAuthCheckReserves127(t *testing.T) {
	s, err := GhAuthCheck("github.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "command -v gh >/dev/null 2>&1 || exit 127") {
		t.Fatalf("gh-auth check must reserve exit 127 for a missing gh: %s", s)
	}
}

func TestRestoreUsesApplyBySHANeverPop(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	s, err := RestoreScript(repo("dotfiles", []string{"main"}), "main", sha)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "stash pop") || strings.Contains(s, "stash@{") {
		t.Fatalf("restore must apply by SHA, never pop or index-address the stash: %s", s)
	}
	if !strings.Contains(s, "stash apply -q "+sha) {
		t.Fatalf("restore must apply the exact carried SHA: %s", s)
	}
}

func TestRestoreRejectsUnvalidatedOrigOrSHA(t *testing.T) {
	r := repo("dotfiles", []string{"main"})
	if _, err := RestoreScript(r, "main; id", ""); err == nil {
		t.Fatal("a shell-metacharacter-laden orig must be rejected")
	}
	if _, err := RestoreScript(r, "main", "abcdef0123456789abcdef0123456789abcdef0"[:39]); err == nil {
		t.Fatal("a 39-char (non-40) SHA must be rejected")
	}
}
