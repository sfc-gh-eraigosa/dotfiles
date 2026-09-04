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
}
