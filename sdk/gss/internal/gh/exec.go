// Package gh wraps the GitHub CLI (`gh`) for the gss codebase.
//
// Every interaction with GitHub goes through this package's Client
// interface. gss never speaks to the GitHub REST/GraphQL API directly and
// never opens a PR with raw git when a gh verb exists for the job — this
// keeps auth, retries, and default-repo resolution consistent with the
// rest of the dotfiles (design.md → "GitHub interaction — gh only";
// resolution #5). Direct use of os/exec to call gh from any layer above
// this package is forbidden by design and will be enforced by a CI grep at
// PR-50 (the same rule that guards internal/git).
//
// Two implementations are provided:
//
//   - SystemClient — shells out to the real `gh` binary on $PATH via the
//     Exec seam. Construct with NewSystemClient() in production code.
//   - fake.Client (sub-package internal/gh/fake) — a stateful, per-verb
//     scriptable fake used in tests.
//
// # Why an Exec seam instead of integration tests
//
// internal/git's SystemRunner is tested against a real `git` in a temp
// repo. gh's mutating verbs (pr create / edit / ready) cannot be tested
// that way: they would touch a live, shared GitHub repo and are not
// reversible. So SystemClient is built over the small Exec interface; its
// unit tests inject a stub Exec to assert (a) the exact gh argv each verb
// builds and (b) that gh's --json output parses correctly. The only logic
// in the real Exec (systemExec) is process spawning and stdout/stderr
// separation.
//
// # Output contract (differs from internal/git on purpose)
//
// git.Runner returns combined stdout+stderr because git's porcelain and
// its error text are both interesting to callers. gh, by contrast, emits
// machine-readable --json on stdout and human chatter on stderr, so the
// Exec seam returns stdout ONLY and folds stderr into the error on a
// non-zero exit (see ExecError). This keeps every parser fed clean JSON.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// Client is the single entry point for GitHub operations in the gss
// codebase. The interface signature is pinned by design.md → "Test seams"
// and must not change without a design review.
type Client interface {
	PRCreate(ctx context.Context, opts PRCreateOpts) (PR, error)
	PREdit(ctx context.Context, num int, opts PREditOpts) error
	PRReady(ctx context.Context, num int) error
	PRView(ctx context.Context, num int) (PR, error)
	PRList(ctx context.Context, filter PRFilter) ([]PR, error)
	RepoView(ctx context.Context) (RepoInfo, error)
	AuthStatus(ctx context.Context) error
}

// Exec abstracts running the gh binary. args is the full gh argv minus the
// binary itself (e.g. "pr", "view", "42", "--json", …). Run returns the
// command's stdout; on a non-zero exit it returns a non-nil error wrapping
// stderr (an *ExecError from systemExec). Tests inject a stub.
type Exec interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// prJSONFields is the fixed --json field set requested by PRView / PRList.
// Keeping it in one place guarantees the two verbs decode into the same PR
// shape.
const prJSONFields = "number,title,body,state,isDraft,mergeable,baseRefName,headRefName,url"

// repoJSONFields is the --json field set requested by RepoView.
const repoJSONFields = "name,owner,nameWithOwner,defaultBranchRef"

// SystemClient is the production Client. It is safe to share across
// goroutines: it holds no mutable state and each call builds its own argv.
type SystemClient struct {
	exec Exec
}

// NewSystemClient returns a SystemClient wired to the real `gh` binary on
// $PATH. This is the canonical production constructor.
func NewSystemClient() *SystemClient {
	return &SystemClient{exec: &systemExec{bin: "gh"}}
}

// NewClientWithExec returns a SystemClient backed by a caller-supplied Exec.
// Production code uses NewSystemClient; this constructor exists so tests can
// inject a scripted Exec and assert argv/parsing without spawning gh.
func NewClientWithExec(e Exec) *SystemClient {
	return &SystemClient{exec: e}
}

// Compile-time interface assertions.
var (
	_ Client = (*SystemClient)(nil)
	_ Exec   = (*systemExec)(nil)
)

// PRCreate opens a pull request and returns its number + URL. Title, Base
// and Head are required. gh pr create prints the new PR's URL on stdout;
// the number is parsed from that URL (gh pr create has no --json mode).
func (c *SystemClient) PRCreate(ctx context.Context, opts PRCreateOpts) (PR, error) {
	if opts.Title == "" || opts.Base == "" || opts.Head == "" {
		return PR{}, fmt.Errorf("gh: PRCreate requires non-empty Title, Base and Head")
	}
	args := []string{"pr", "create", "--title", opts.Title, "--base", opts.Base, "--head", opts.Head}
	if opts.Draft {
		args = append(args, "--draft")
	}
	args = append(args, bodyArgs(opts.BodyFile, opts.Body, true)...)

	out, err := c.exec.Run(ctx, args...)
	if err != nil {
		return PR{}, fmt.Errorf("gh pr create: %w", err)
	}
	num, url, err := parseCreatedPR(out)
	if err != nil {
		return PR{}, err
	}
	return PR{
		Number:  num,
		URL:     url,
		Title:   opts.Title,
		Base:    opts.Base,
		Head:    opts.Head,
		IsDraft: opts.Draft,
		State:   "OPEN",
	}, nil
}

// PREdit updates a PR's base and/or body. num must be > 0 and at least one
// of Base / Body / BodyFile must be set.
func (c *SystemClient) PREdit(ctx context.Context, num int, opts PREditOpts) error {
	if num <= 0 {
		return fmt.Errorf("gh: PREdit requires num > 0, got %d", num)
	}
	args := []string{"pr", "edit", strconv.Itoa(num)}
	if opts.Base != "" {
		args = append(args, "--base", opts.Base)
	}
	args = append(args, bodyArgs(opts.BodyFile, opts.Body, false)...)
	if len(args) == 3 {
		return fmt.Errorf("gh: PREdit requires at least one of Base, Body, BodyFile")
	}
	if _, err := c.exec.Run(ctx, args...); err != nil {
		return fmt.Errorf("gh pr edit %d: %w", num, err)
	}
	return nil
}

// PRReady promotes a draft PR to ready-for-review.
func (c *SystemClient) PRReady(ctx context.Context, num int) error {
	if num <= 0 {
		return fmt.Errorf("gh: PRReady requires num > 0, got %d", num)
	}
	if _, err := c.exec.Run(ctx, "pr", "ready", strconv.Itoa(num)); err != nil {
		return fmt.Errorf("gh pr ready %d: %w", num, err)
	}
	return nil
}

// PRView returns the current state of a single PR.
func (c *SystemClient) PRView(ctx context.Context, num int) (PR, error) {
	if num <= 0 {
		return PR{}, fmt.Errorf("gh: PRView requires num > 0, got %d", num)
	}
	out, err := c.exec.Run(ctx, "pr", "view", strconv.Itoa(num), "--json", prJSONFields)
	if err != nil {
		return PR{}, fmt.Errorf("gh pr view %d: %w", num, err)
	}
	return ParsePR(out)
}

// PRList returns PRs matching filter. The zero filter lists OPEN PRs.
func (c *SystemClient) PRList(ctx context.Context, filter PRFilter) ([]PR, error) {
	state := filter.State
	if state == "" {
		state = "open"
	}
	args := []string{"pr", "list", "--state", state, "--json", prJSONFields}
	if filter.Head != "" {
		args = append(args, "--head", filter.Head)
	}
	if filter.Base != "" {
		args = append(args, "--base", filter.Base)
	}
	if filter.Limit > 0 {
		args = append(args, "--limit", strconv.Itoa(filter.Limit))
	}
	out, err := c.exec.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	return ParsePRs(out)
}

// RepoView resolves the current repository's owner/name/NWO/default branch.
func (c *SystemClient) RepoView(ctx context.Context) (RepoInfo, error) {
	out, err := c.exec.Run(ctx, "repo", "view", "--json", repoJSONFields)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("gh repo view: %w", err)
	}
	return ParseRepoInfo(out)
}

// AuthStatus reports whether the current gh identity is authenticated. A
// non-zero `gh auth status` is mapped to errors.ErrGHAuthRequired so
// callers can errors.Is the well-known sentinel (and the exit-code map
// resolves it) rather than matching on gh's free-text output.
func (c *SystemClient) AuthStatus(ctx context.Context) error {
	if _, err := c.exec.Run(ctx, "auth", "status"); err != nil {
		return fmt.Errorf("%w: %v", errors.ErrGHAuthRequired, err)
	}
	return nil
}

// bodyArgs builds the body flag(s) for create/edit. BodyFile wins over
// Body (design prefers --body-file). For create, an absent body still emits
// an explicit empty --body so gh does not drop into an interactive editor
// in a non-tty agent context; for edit, an absent body emits nothing so the
// existing body is preserved.
func bodyArgs(bodyFile, body string, forCreate bool) []string {
	switch {
	case bodyFile != "":
		return []string{"--body-file", bodyFile}
	case body != "":
		return []string{"--body", body}
	case forCreate:
		return []string{"--body", ""}
	default:
		return nil
	}
}

// prWire is the on-wire shape of a gh PR object. It is decoded then mapped
// to the public PR so the domain type stays free of gh's camelCase keys.
type prWire struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	Mergeable   string `json:"mergeable"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	URL         string `json:"url"`
}

func (w prWire) toPR() PR {
	return PR{
		Number:    w.Number,
		Title:     w.Title,
		Body:      w.Body,
		State:     w.State,
		IsDraft:   w.IsDraft,
		Mergeable: w.Mergeable,
		Base:      w.BaseRefName,
		Head:      w.HeadRefName,
		URL:       w.URL,
	}
}

// ParsePR decodes a single `gh pr view --json …` object into a PR. Exported
// so the fake can seed its state from the same testdata fixtures the
// SystemClient parses.
func ParsePR(data []byte) (PR, error) {
	var w prWire
	if err := json.Unmarshal(data, &w); err != nil {
		return PR{}, fmt.Errorf("gh: parse PR: %w", err)
	}
	return w.toPR(), nil
}

// ParsePRs decodes a `gh pr list --json …` array into a slice of PR.
func ParsePRs(data []byte) ([]PR, error) {
	var ws []prWire
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("gh: parse PR list: %w", err)
	}
	prs := make([]PR, len(ws))
	for i, w := range ws {
		prs[i] = w.toPR()
	}
	return prs, nil
}

// repoWire is the on-wire shape of a gh repo object, flattening the nested
// owner.login and defaultBranchRef.name into RepoInfo.
type repoWire struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	NameWithOwner    string `json:"nameWithOwner"`
	DefaultBranchRef struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

// ParseRepoInfo decodes a `gh repo view --json …` object into RepoInfo.
func ParseRepoInfo(data []byte) (RepoInfo, error) {
	var w repoWire
	if err := json.Unmarshal(data, &w); err != nil {
		return RepoInfo{}, fmt.Errorf("gh: parse repo info: %w", err)
	}
	return RepoInfo{
		Owner:         w.Owner.Login,
		Name:          w.Name,
		NameWithOwner: w.NameWithOwner,
		DefaultBranch: w.DefaultBranchRef.Name,
	}, nil
}

// prURLRe extracts the PR number from a github.com/<owner>/<repo>/pull/<n>
// URL, which is all `gh pr create` prints on success.
var prURLRe = regexp.MustCompile(`/pull/(\d+)`)

// parseCreatedPR pulls the PR number and URL out of `gh pr create` stdout.
// gh prints the URL on its own line; we scan for the first /pull/<n> match
// to stay robust to any leading progress text.
func parseCreatedPR(out []byte) (int, string, error) {
	m := prURLRe.FindSubmatch(out)
	if m == nil {
		return 0, "", fmt.Errorf("gh pr create: could not find a /pull/<n> URL in output: %q", bytes.TrimSpace(out))
	}
	num, _ := strconv.Atoi(string(m[1]))
	url := ""
	for _, f := range strings.Fields(string(out)) {
		if prURLRe.MatchString(f) {
			url = f
			break
		}
	}
	return num, url, nil
}

// systemExec is the production Exec: it spawns the real gh binary and
// separates stdout (returned) from stderr (folded into ExecError).
type systemExec struct {
	// bin overrides the gh binary location; empty defaults to "gh" on
	// $PATH. Tests of higher layers never reach this — they inject a stub
	// Exec — but keeping bin configurable mirrors git.SystemRunner.Path.
	bin string
}

func (s *systemExec) Run(ctx context.Context, args ...string) ([]byte, error) {
	bin := s.bin
	if bin == "" {
		bin = "gh"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &ExecError{Args: args, Stderr: errb.Bytes(), Err: err}
	}
	return out.Bytes(), nil
}

// ExecError wraps a failed gh invocation, preserving the argv and gh's
// stderr so callers (and AuthStatus's sentinel mapping) can surface a
// useful message. Err is the underlying *exec.ExitError / lookup error and
// is exposed via Unwrap for errors.As.
type ExecError struct {
	Args   []string
	Stderr []byte
	Err    error
}

func (e *ExecError) Error() string {
	if s := bytes.TrimSpace(e.Stderr); len(s) > 0 {
		return fmt.Sprintf("gh %s: %v: %s", strings.Join(e.Args, " "), e.Err, s)
	}
	return fmt.Sprintf("gh %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *ExecError) Unwrap() error { return e.Err }
