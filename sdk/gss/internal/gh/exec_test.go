// Package gh_test verifies the SystemClient against a scripted Exec seam
// and exercises the JSON parsers. These are TDD-first proof for PR-03:
// when this file lands without exec.go/pr.go/repo.go the package fails to
// compile (the gh.Client / gh.SystemClient symbols are undefined). Once
// the implementation ships, every case here is expected to pass.
//
// Unlike internal/git's PR-02 tests, we do NOT shell out to the real gh
// binary: gh's mutating verbs (pr create / edit / ready) would touch a
// live GitHub repo, which is neither hermetic nor reversible. Instead the
// SystemClient is constructed over an injected Exec stub so we can assert
// (a) the exact gh argv each verb builds and (b) that the verb parses gh's
// --json output correctly. The real gh wiring (systemExec) is a thin
// os/exec shell whose only logic is stdout/stderr separation.
package gh_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
)

// stubExec is a scripted gh.Exec: it records every argv it is handed and
// returns the next (stdout, err) pair from its scripts. Exhausted scripts
// return (nil, nil) so callers that don't care about output stay terse.
type stubExec struct {
	calls [][]string
	out   [][]byte
	errs  []error
	i     int
}

func (s *stubExec) Run(_ context.Context, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string(nil), args...))
	var out []byte
	var err error
	if s.i < len(s.out) {
		out = s.out[s.i]
	}
	if s.i < len(s.errs) {
		err = s.errs[s.i]
	}
	s.i++
	return out, err
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "gh_responses", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestSystemClient_ImplementsClient is a compile-time assertion that the
// production client satisfies the pinned interface. If the interface
// signature in design.md drifts from the implementation, this fails to
// build.
func TestSystemClient_ImplementsClient(t *testing.T) {
	var _ gh.Client = (*gh.SystemClient)(nil)
	var _ gh.Client = gh.NewSystemClient()
}

func TestPRCreate_ArgsAndParse(t *testing.T) {
	stub := &stubExec{out: [][]byte{[]byte("https://github.com/sfc-gh-eraigosa/playground/pull/42\n")}}
	c := gh.NewClientWithExec(stub)

	pr, err := c.PRCreate(context.Background(), gh.PRCreateOpts{
		Title:    "PR-03: internal/gh",
		BodyFile: "/tmp/body.md",
		Base:     "pr02-internal-git-runner",
		Head:     "pr03-internal-gh-client",
		Draft:    true,
	})
	if err != nil {
		t.Fatalf("PRCreate: %v", err)
	}

	wantArgs := []string{
		"pr", "create",
		"--title", "PR-03: internal/gh",
		"--base", "pr02-internal-git-runner",
		"--head", "pr03-internal-gh-client",
		"--draft",
		"--body-file", "/tmp/body.md",
	}
	if !reflect.DeepEqual(stub.calls[0], wantArgs) {
		t.Errorf("argv =\n  %q\nwant\n  %q", stub.calls[0], wantArgs)
	}
	if pr.Number != 42 {
		t.Errorf("PR.Number = %d; want 42 (parsed from create URL)", pr.Number)
	}
	if pr.URL != "https://github.com/sfc-gh-eraigosa/playground/pull/42" {
		t.Errorf("PR.URL = %q", pr.URL)
	}
	if !pr.IsDraft || pr.Base != "pr02-internal-git-runner" || pr.Head != "pr03-internal-gh-client" {
		t.Errorf("PR echo fields wrong: %+v", pr)
	}
}

func TestPRCreate_InlineBodyAndNoDraft(t *testing.T) {
	stub := &stubExec{out: [][]byte{[]byte("https://github.com/o/r/pull/7\n")}}
	c := gh.NewClientWithExec(stub)

	if _, err := c.PRCreate(context.Background(), gh.PRCreateOpts{
		Title: "t", Base: "main", Head: "feat", Body: "inline body",
	}); err != nil {
		t.Fatalf("PRCreate: %v", err)
	}
	got := stub.calls[0]
	if contains(got, "--draft") {
		t.Errorf("non-draft create should not pass --draft: %q", got)
	}
	if !adjacent(got, "--body", "inline body") {
		t.Errorf("inline body should be passed as --body: %q", got)
	}
}

func TestPRCreate_EmptyInputRejected(t *testing.T) {
	stub := &stubExec{}
	c := gh.NewClientWithExec(stub)
	if _, err := c.PRCreate(context.Background(), gh.PRCreateOpts{Base: "main", Head: "feat"}); err == nil {
		t.Error("PRCreate with empty Title: err = nil; want validation error")
	}
	if len(stub.calls) != 0 {
		t.Errorf("PRCreate should reject before exec; got %d calls", len(stub.calls))
	}
}

func TestPRView_ParsesJSON(t *testing.T) {
	stub := &stubExec{out: [][]byte{fixture(t, "pr_view_draft.json")}}
	c := gh.NewClientWithExec(stub)

	pr, err := c.PRView(context.Background(), 24)
	if err != nil {
		t.Fatalf("PRView: %v", err)
	}
	if stub.calls[0][0] != "pr" || stub.calls[0][1] != "view" || stub.calls[0][2] != "24" {
		t.Errorf("argv prefix = %q; want [pr view 24 ...]", stub.calls[0])
	}
	if !contains(stub.calls[0], "--json") {
		t.Errorf("PRView must request --json: %q", stub.calls[0])
	}
	if pr.Number != 24 || !pr.IsDraft || pr.Base != "pr01-internal-errors" ||
		pr.Head != "pr02-internal-git-runner" || pr.State != "OPEN" {
		t.Errorf("parsed PR wrong: %+v", pr)
	}
}

func TestPRView_RejectsBadNum(t *testing.T) {
	stub := &stubExec{}
	c := gh.NewClientWithExec(stub)
	if _, err := c.PRView(context.Background(), 0); err == nil {
		t.Error("PRView(0): err = nil; want validation error")
	}
	if len(stub.calls) != 0 {
		t.Errorf("PRView(0) should reject before exec; got %d calls", len(stub.calls))
	}
}

func TestPRList_ParsesArray(t *testing.T) {
	stub := &stubExec{out: [][]byte{fixture(t, "pr_list.json")}}
	c := gh.NewClientWithExec(stub)

	prs, err := c.PRList(context.Background(), gh.PRFilter{State: "all", Limit: 50})
	if err != nil {
		t.Fatalf("PRList: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d; want 2", len(prs))
	}
	if prs[0].Number != 23 || prs[1].Number != 24 {
		t.Errorf("PR numbers = %d,%d; want 23,24", prs[0].Number, prs[1].Number)
	}
	if !adjacent(stub.calls[0], "--state", "all") {
		t.Errorf("PRList should pass --state all: %q", stub.calls[0])
	}
	if !adjacent(stub.calls[0], "--limit", "50") {
		t.Errorf("PRList should pass --limit 50: %q", stub.calls[0])
	}
}

func TestPRList_DefaultStateOpen(t *testing.T) {
	stub := &stubExec{out: [][]byte{[]byte("[]")}}
	c := gh.NewClientWithExec(stub)
	if _, err := c.PRList(context.Background(), gh.PRFilter{}); err != nil {
		t.Fatalf("PRList: %v", err)
	}
	if !adjacent(stub.calls[0], "--state", "open") {
		t.Errorf("empty filter should default --state open: %q", stub.calls[0])
	}
}

func TestPREdit_Args(t *testing.T) {
	stub := &stubExec{}
	c := gh.NewClientWithExec(stub)
	if err := c.PREdit(context.Background(), 24, gh.PREditOpts{Base: "test_gss"}); err != nil {
		t.Fatalf("PREdit: %v", err)
	}
	want := []string{"pr", "edit", "24", "--base", "test_gss"}
	if !reflect.DeepEqual(stub.calls[0], want) {
		t.Errorf("argv = %q; want %q", stub.calls[0], want)
	}
}

func TestPREdit_EmptyInputRejected(t *testing.T) {
	c := gh.NewClientWithExec(&stubExec{})
	if err := c.PREdit(context.Background(), 0, gh.PREditOpts{Base: "main"}); err == nil {
		t.Error("PREdit(0): err = nil; want validation error")
	}
	if err := c.PREdit(context.Background(), 24, gh.PREditOpts{}); err == nil {
		t.Error("PREdit with no fields: err = nil; want validation error")
	}
}

func TestPRReady_Args(t *testing.T) {
	stub := &stubExec{}
	c := gh.NewClientWithExec(stub)
	if err := c.PRReady(context.Background(), 24); err != nil {
		t.Fatalf("PRReady: %v", err)
	}
	want := []string{"pr", "ready", "24"}
	if !reflect.DeepEqual(stub.calls[0], want) {
		t.Errorf("argv = %q; want %q", stub.calls[0], want)
	}
}

func TestRepoView_ParsesNested(t *testing.T) {
	stub := &stubExec{out: [][]byte{fixture(t, "repo_view.json")}}
	c := gh.NewClientWithExec(stub)
	info, err := c.RepoView(context.Background())
	if err != nil {
		t.Fatalf("RepoView: %v", err)
	}
	if info.Owner != "sfc-gh-eraigosa" || info.Name != "playground" ||
		info.NameWithOwner != "sfc-gh-eraigosa/playground" || info.DefaultBranch != "main" {
		t.Errorf("RepoInfo wrong: %+v", info)
	}
}

func TestAuthStatus_OK(t *testing.T) {
	c := gh.NewClientWithExec(&stubExec{})
	if err := c.AuthStatus(context.Background()); err != nil {
		t.Errorf("AuthStatus (logged in): err = %v; want nil", err)
	}
}

func TestAuthStatus_NotLoggedIn(t *testing.T) {
	stub := &stubExec{errs: []error{stderrors.New("not logged in")}}
	c := gh.NewClientWithExec(stub)
	err := c.AuthStatus(context.Background())
	if err == nil {
		t.Fatal("AuthStatus (logged out): err = nil; want ErrGHAuthRequired")
	}
	if !stderrors.Is(err, errors.ErrGHAuthRequired) {
		t.Errorf("err = %v; want wrapping errors.ErrGHAuthRequired", err)
	}
}

func TestPRView_PropagatesExecError(t *testing.T) {
	stub := &stubExec{errs: []error{stderrors.New("gh: PR not found")}}
	c := gh.NewClientWithExec(stub)
	if _, err := c.PRView(context.Background(), 999); err == nil {
		t.Error("PRView with exec error: err = nil; want propagated error")
	}
}

// Parser-only tests (no exec) — these double as documentation of the gh
// --json field mapping and keep coverage on the parse path even when the
// verb wiring changes.

func TestParsePR(t *testing.T) {
	pr, err := gh.ParsePR(fixture(t, "pr_view_ready.json"))
	if err != nil {
		t.Fatalf("ParsePR: %v", err)
	}
	if pr.Number != 24 || pr.IsDraft || pr.Base != "test_gss" || pr.Mergeable != "MERGEABLE" {
		t.Errorf("ParsePR wrong: %+v", pr)
	}
}

func TestParsePR_Invalid(t *testing.T) {
	if _, err := gh.ParsePR([]byte("not json")); err == nil {
		t.Error("ParsePR(garbage): err = nil; want decode error")
	}
}

func TestParseRepoInfo(t *testing.T) {
	info, err := gh.ParseRepoInfo(fixture(t, "repo_view.json"))
	if err != nil {
		t.Fatalf("ParseRepoInfo: %v", err)
	}
	if info.DefaultBranch != "main" || info.Owner != "sfc-gh-eraigosa" {
		t.Errorf("ParseRepoInfo wrong: %+v", info)
	}
}

// TestExecError pins the error-output contract: ExecError surfaces the
// argv and gh's stderr, and Unwrap exposes the underlying error so callers
// can errors.As/Is through it. (gh analog of carry-forward note #7 —
// pinning what the wrapper folds into the error vs. returns as stdout.)
func TestExecError(t *testing.T) {
	underlying := stderrors.New("exit status 1")
	e := &gh.ExecError{
		Args:   []string{"pr", "view", "999"},
		Stderr: []byte("  could not resolve to a PullRequest\n"),
		Err:    underlying,
	}
	msg := e.Error()
	for _, want := range []string{"pr view 999", "could not resolve to a PullRequest", "exit status 1"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ExecError.Error() = %q; want substring %q", msg, want)
		}
	}
	if !stderrors.Is(e, underlying) {
		t.Errorf("errors.Is(ExecError, underlying) = false; want true via Unwrap")
	}

	// With empty stderr, the message omits the trailing ": <stderr>" tail.
	bare := (&gh.ExecError{Args: []string{"auth", "status"}, Err: underlying}).Error()
	if strings.Contains(bare, "could not resolve") {
		t.Errorf("bare ExecError leaked stderr: %q", bare)
	}
}

// --- tiny argv assertion helpers (kept at the bottom for readability) ---

func contains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

// adjacent reports whether flag is immediately followed by val in argv.
func adjacent(argv []string, flag, val string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == val {
			return true
		}
	}
	return false
}
