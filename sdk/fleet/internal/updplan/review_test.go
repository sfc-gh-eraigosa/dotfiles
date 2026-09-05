package updplan

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

// Findings from the leaf A code review (PR #270). Each test names the defect
// it pins so a regression is self-explaining.

// A ref that starts with '-' is a git OPTION once interpolated bare into
// `git fetch origin <ref>`; `--upload-pack` would even execute a program.
// git check-ref-format already refuses these names, so nothing legitimate is lost.
func TestValidRefRejectsOptionLookalikesAndBadRefFormat(t *testing.T) {
	for _, bad := range []string{"-q", "--upload-pack", "a..b", "feat@{1}", "x.lock", "-"} {
		if ValidRef(bad) {
			t.Errorf("ValidRef accepted %q, which git check-ref-format rejects", bad)
		}
	}
	for _, good := range []string{"main", "release-1.2", "feature/x-y", "v1.2.3", "a.b"} {
		if !ValidRef(good) {
			t.Errorf("ValidRef rejected legitimate %q", good)
		}
	}
}

// WithRef must not mint a plan Parse itself would reject.
func TestWithRefRevalidatesTheBranchList(t *testing.T) {
	p := parseOK(t, `
version: 1
update:
  repos:
    dotfiles: {branches: [main, develop]}
  steps:
    - {id: s, kind: sync, repo: dotfiles}
`)
	if _, err := p.WithRef("-q"); err == nil {
		t.Fatal("WithRef accepted an option-lookalike ref")
	}
	q, err := p.WithRef("hotfix")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Repos["dotfiles"].Branches; strings.Join(got, ",") != "hotfix,develop" {
		t.Fatalf("branches = %v", got)
	}
}

// update.root is prefixed onto every relative repo path, so it must obey the
// same charset rule as path: AND be absolute or ~/-relative — otherwise
// Repo.Path ends up relative to the ssh login cwd (or carries a metacharacter).
func TestRootIsValidatedAndMustBeAbsoluteOrHome(t *testing.T) {
	for _, root := range []string{"git", `"$(rm -rf ~); echo"`, "../x", "-rf"} {
		_, err := Parse([]byte("version: 1\nupdate:\n  root: " + root + "\n  repos:\n    x: {}\n"))
		if err == nil || !strings.Contains(err.Error(), "root") {
			t.Errorf("root %q: want a root validation error, got %v", root, err)
		}
	}
	for _, root := range []string{"~/git", "/srv/repos", "~"} {
		p, err := Parse([]byte("version: 1\nupdate:\n  root: " + root + "\n  repos:\n    x: {}\n"))
		if err != nil {
			t.Errorf("root %q rejected: %v", root, err)
			continue
		}
		if got := p.Repos["x"].Path; strings.HasPrefix(got, ".") || (!strings.HasPrefix(got, "/") && !strings.HasPrefix(got, "~")) {
			t.Errorf("root %q: resolved path %q is not absolute or ~-relative", root, got)
		}
	}
}

// Every exit-code token normalises to exit:<n> so the executor can match it.
func TestRetryOnExitTokensAreStrictAndNormalised(t *testing.T) {
	p := parseOK(t, `
version: 1
update:
  defaults: {retry: {on: ["exit:3", 7]}}
  repos: {x: {}}
  steps: [{id: s, kind: sync, repo: x}]
`)
	if got := p.Defaults.Retry.On; len(got) != 2 || got[0] != "exit:3" || got[1] != "exit:7" {
		t.Fatalf("On = %v", got)
	}
	for _, bad := range []string{`"exit: 3"`, `"exit:07"`, `"exit:12abc"`, `"exit:0x1f"`, `"exit:256"`, `"exit:-1"`} {
		_, err := Parse([]byte("version: 1\nupdate:\n  defaults: {retry: {on: [" + bad + "]}}\n  repos: {x: {}}\n  steps: [{id: s, kind: sync, repo: x}]\n"))
		if err == nil {
			t.Errorf("retry.on %s accepted", bad)
		}
	}
}

// The cap is applied in float space, so an overflowed power never becomes a
// negative Duration; NaN/Inf factors are rejected at parse.
func TestBackoffWaitNeverOverflowsAndRejectsNaN(t *testing.T) {
	b := Backoff{Initial: 5 * time.Second, Factor: 1e300, Max: 2 * time.Minute, Jitter: true}
	for n := 1; n <= 40; n++ {
		if d := b.Wait(n, func() float64 { return 1 }); d < 0 || d > 3*time.Minute {
			t.Fatalf("n=%d: wait %v out of range", n, d)
		}
	}
	if d := (Backoff{Initial: 5 * time.Second, Factor: 2, Max: 0}).Wait(64, nil); d < 0 || d > time.Duration(math.MaxInt64) {
		t.Fatalf("uncapped overflow produced %v", d)
	}
	for _, f := range []string{".nan", ".inf"} {
		_, err := Parse([]byte("version: 1\nupdate:\n  defaults: {retry: {backoff: {factor: " + f + "}}}\n  repos: {x: {}}\n  steps: [{id: s, kind: sync, repo: x}]\n"))
		if err == nil {
			t.Errorf("factor %s accepted", f)
		}
	}
}

// Tags cannot be told from branches syntactically; the old heuristic rejected
// real branches like v2 and let real tags through. Multi-branch lists are
// documented as branches-only and no longer guessed at.
func TestBranchListsAreNotJudgedByNameHeuristics(t *testing.T) {
	p := parseOK(t, `
version: 1
update:
  repos: {x: {branches: [main, v2]}}
  steps: [{id: s, kind: sync, repo: x}]
`)
	if got := p.Repos["x"].Branches; strings.Join(got, ",") != "main,v2" {
		t.Fatalf("branches = %v", got)
	}
}

// Default() hands out a value callers may mutate; it must never leak into the
// next caller's plan.
func TestDefaultIsNotSharedBetweenCallers(t *testing.T) {
	d := Default()
	d.Repos["dotfiles"] = Repo{Name: "mutated"}
	d.Steps[0].Timeout = 99 * time.Hour
	d.Steps[0].Needs = append(d.Steps[0].Needs, "x")
	fresh := Default()
	if fresh.Repos["dotfiles"].Name != "dotfiles" || fresh.Steps[0].Timeout == 99*time.Hour || len(fresh.Steps[0].Needs) != 0 {
		t.Fatalf("Default() leaked a mutation: %+v", fresh)
	}
}

// hostname is a gh-auth-only field (spec F1), like run: is run-only.
func TestHostnameIsRejectedOnNonGhAuthSteps(t *testing.T) {
	_, err := Parse([]byte("version: 1\nupdate:\n  repos: {x: {}}\n  steps: [{id: s, kind: sync, repo: x, hostname: 'evil host'}]\n"))
	if err == nil || !strings.Contains(err.Error(), "hostname") {
		t.Fatalf("want a hostname error, got %v", err)
	}
}

// An unknown kind must produce ONE error about kind, not a second misleading
// one about run:.
func TestUnknownKindDoesNotCascadeIntoRunErrors(t *testing.T) {
	_, err := Parse([]byte("version: 1\nupdate:\n  repos: {x: {}}\n  steps: [{id: a, kind: bogus, run: echo hi}]\n"))
	if err == nil {
		t.Fatal("unknown kind accepted")
	}
	if strings.Contains(err.Error(), "only run steps may set run") {
		t.Fatalf("kind error cascaded into a run: error: %v", err)
	}
}

// An empty/comment-only file is a schema problem, not an I/O one; a second
// YAML document is refused rather than silently dropped.
func TestEmptyAndMultiDocumentFilesAreExplicitErrors(t *testing.T) {
	for _, in := range []string{"", "# only a comment\n"} {
		_, err := Parse([]byte(in))
		if err == nil || errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "empty") {
			t.Errorf("input %q: want an explicit empty-plan error, got %v", in, err)
		}
	}
	_, err := Parse([]byte("version: 1\nupdate:\n  repos: {x: {}}\n  steps: [{id: s, kind: sync, repo: x}]\n---\nversion: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "document") {
		t.Fatalf("want a multi-document error, got %v", err)
	}
}

func parseOK(t *testing.T, yaml string) Plan {
	t.Helper()
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}
