// Package fake provides a stateful, scriptable implementation of gh.Client
// for use in tests.
//
// Per design.md ("Test seams"), every package that talks to GitHub does so
// through gh.Client. Production wires gh.SystemClient; tests wire
// fake.Client. The fake keeps an in-memory set of PRs and mutates it as the
// verbs are called, so an orchestrator test can assert a whole conversation
// — "create a draft, promote it, view it, confirm it's ready" — against a
// single fake with no network.
//
// # Per-verb scripting (vs. the git fake's global FIFO)
//
// internal/git/fake uses one global FIFO Script because git callers run a
// linear sequence of subcommands. gh callers interleave verbs (create here,
// view there, list elsewhere), so a single FIFO would be brittle: scripting
// "the third call fails" couples unrelated verbs. Instead the gh fake
// scripts errors PER VERB (carry-forward note #9). ScriptError(VerbPRView,
// err) makes the next PRView fail without touching PRCreate's behaviour.
// When a verb's error queue is empty it performs its normal stateful work.
//
// # Concurrency
//
// All state is guarded by a single mutex; the fake is safe to share across
// goroutines, which matters for future fan-out commands such as
// `gss feature checkpoint --auto`.
package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
)

// Verb names for ScriptError / Call.Verb. They mirror the gh.Client method
// names so test setup reads naturally.
const (
	VerbPRCreate   = "PRCreate"
	VerbPREdit     = "PREdit"
	VerbPRReady    = "PRReady"
	VerbPRView     = "PRView"
	VerbPRList     = "PRList"
	VerbRepoView   = "RepoView"
	VerbAuthStatus = "AuthStatus"
)

// Call records one invocation. Num is set for the num-bearing verbs;
// CreateOpts / EditOpts / Filter carry the typed argument for the verb that
// uses them so tests can assert on what was passed.
type Call struct {
	Verb       string
	Num        int
	CreateOpts gh.PRCreateOpts
	EditOpts   gh.PREditOpts
	Filter     gh.PRFilter
}

// Client is a stateful, per-verb scriptable gh.Client fake.
type Client struct {
	mu        sync.Mutex
	repo      gh.RepoInfo
	authErr   error
	prs       map[int]gh.PR
	nextNum   int
	calls     []Call
	errScript map[string][]error
}

// NewClient returns a ready-to-use fake with an empty PR set. The first PR
// minted by PRCreate gets number 1.
func NewClient() *Client {
	return &Client{
		prs:       make(map[int]gh.PR),
		nextNum:   1,
		errScript: make(map[string][]error),
	}
}

// Compile-time proof the fake satisfies the interface (carry-forward #4).
var _ gh.Client = (*Client)(nil)

// PRCreate mints a numbered, OPEN PR from opts and stores it. Title, Base
// and Head are required (empty-input contract, carry-forward #5).
func (c *Client) PRCreate(_ context.Context, opts gh.PRCreateOpts) (gh.PR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbPRCreate, CreateOpts: opts})
	if err := c.popErr(VerbPRCreate); err != nil {
		return gh.PR{}, err
	}
	if opts.Title == "" || opts.Base == "" || opts.Head == "" {
		return gh.PR{}, fmt.Errorf("fake gh: PRCreate requires non-empty Title, Base and Head")
	}
	num := c.nextNum
	c.nextNum++
	pr := gh.PR{
		Number:    num,
		Title:     opts.Title,
		Body:      bodyOf(opts.BodyFile, opts.Body),
		State:     "OPEN",
		IsDraft:   opts.Draft,
		Mergeable: "MERGEABLE",
		Base:      opts.Base,
		Head:      opts.Head,
		URL:       fmt.Sprintf("https://github.com/%s/pull/%d", c.nwo(), num),
	}
	c.prs[num] = pr
	return pr, nil
}

// PREdit rewrites the stored PR's base and/or body. Empty fields are left
// unchanged. num must be > 0, at least one field must be set, and the PR
// must exist.
func (c *Client) PREdit(_ context.Context, num int, opts gh.PREditOpts) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbPREdit, Num: num, EditOpts: opts})
	if err := c.popErr(VerbPREdit); err != nil {
		return err
	}
	if num <= 0 {
		return fmt.Errorf("fake gh: PREdit requires num > 0, got %d", num)
	}
	if opts.Base == "" && opts.Body == "" && opts.BodyFile == "" {
		return fmt.Errorf("fake gh: PREdit requires at least one of Base, Body, BodyFile")
	}
	pr, ok := c.prs[num]
	if !ok {
		return fmt.Errorf("fake gh: PREdit: PR #%d not found", num)
	}
	if opts.Base != "" {
		pr.Base = opts.Base
	}
	if body := bodyOf(opts.BodyFile, opts.Body); body != "" {
		pr.Body = body
	}
	c.prs[num] = pr
	return nil
}

// PRReady flips the stored PR's draft flag off. num must be > 0 and exist.
func (c *Client) PRReady(_ context.Context, num int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbPRReady, Num: num})
	if err := c.popErr(VerbPRReady); err != nil {
		return err
	}
	if num <= 0 {
		return fmt.Errorf("fake gh: PRReady requires num > 0, got %d", num)
	}
	pr, ok := c.prs[num]
	if !ok {
		return fmt.Errorf("fake gh: PRReady: PR #%d not found", num)
	}
	pr.IsDraft = false
	c.prs[num] = pr
	return nil
}

// PRView returns the stored PR. num must be > 0 and exist.
func (c *Client) PRView(_ context.Context, num int) (gh.PR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbPRView, Num: num})
	if err := c.popErr(VerbPRView); err != nil {
		return gh.PR{}, err
	}
	if num <= 0 {
		return gh.PR{}, fmt.Errorf("fake gh: PRView requires num > 0, got %d", num)
	}
	pr, ok := c.prs[num]
	if !ok {
		return gh.PR{}, fmt.Errorf("fake gh: PRView: PR #%d not found", num)
	}
	return pr, nil
}

// PRList returns stored PRs matching filter, sorted ascending by number for
// deterministic assertions. The zero filter (State "") lists OPEN PRs,
// mirroring gh's default and the SystemClient.
func (c *Client) PRList(_ context.Context, filter gh.PRFilter) ([]gh.PR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbPRList, Filter: filter})
	if err := c.popErr(VerbPRList); err != nil {
		return nil, err
	}
	state := filter.State
	if state == "" {
		state = "open"
	}
	var out []gh.PR
	for _, pr := range c.prs {
		if !matchesState(pr.State, state) {
			continue
		}
		if filter.Head != "" && pr.Head != filter.Head {
			continue
		}
		if filter.Base != "" && pr.Base != filter.Base {
			continue
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// RepoView returns the injected RepoInfo (see SetRepo).
func (c *Client) RepoView(_ context.Context) (gh.RepoInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbRepoView})
	if err := c.popErr(VerbRepoView); err != nil {
		return gh.RepoInfo{}, err
	}
	return c.repo, nil
}

// AuthStatus returns the injected auth error (nil = authenticated; see
// SetAuthErr). A per-verb script, if set, takes precedence.
func (c *Client) AuthStatus(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, Call{Verb: VerbAuthStatus})
	if err := c.popErr(VerbAuthStatus); err != nil {
		return err
	}
	return c.authErr
}

// --- test-facing controls ---

// SeedPR inserts or replaces a PR in the fake's state and advances the
// number counter past it so later PRCreate calls don't collide.
func (c *Client) SeedPR(pr gh.PR) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prs[pr.Number] = pr
	if pr.Number >= c.nextNum {
		c.nextNum = pr.Number + 1
	}
}

// SeedFromJSON seeds a PR from a `gh pr view --json …` fixture, using the
// same parser the SystemClient uses (plan PR-03: scriptable from
// testdata/gh_responses/*.json).
func (c *Client) SeedFromJSON(data []byte) error {
	pr, err := gh.ParsePR(data)
	if err != nil {
		return err
	}
	c.SeedPR(pr)
	return nil
}

// SetRepo sets the value returned by RepoView.
func (c *Client) SetRepo(info gh.RepoInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repo = info
}

// SetAuthErr sets the value returned by AuthStatus (nil = authenticated).
func (c *Client) SetAuthErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authErr = err
}

// ScriptError enqueues one or more errors to be returned by the next
// call(s) to verb, in order (FIFO within the verb). Once drained, the verb
// resumes its normal stateful behaviour.
func (c *Client) ScriptError(verb string, errs ...error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errScript[verb] = append(c.errScript[verb], errs...)
}

// Calls returns a copy of the recorded call log in invocation order.
func (c *Client) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Call(nil), c.calls...)
}

// PRCount returns the number of PRs currently held.
func (c *Client) PRCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.prs)
}

// Reset returns the fake to a freshly-constructed state.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prs = make(map[int]gh.PR)
	c.nextNum = 1
	c.calls = nil
	c.errScript = make(map[string][]error)
	c.repo = gh.RepoInfo{}
	c.authErr = nil
}

// popErr returns and consumes the next scripted error for verb. Caller must
// hold the mutex.
func (c *Client) popErr(verb string) error {
	q := c.errScript[verb]
	if len(q) == 0 {
		return nil
	}
	err := q[0]
	c.errScript[verb] = q[1:]
	return err
}

// nwo returns the owner/repo for synthesised PR URLs, falling back to a
// placeholder when RepoView state hasn't been seeded. Caller must hold the
// mutex.
func (c *Client) nwo() string {
	if c.repo.NameWithOwner != "" {
		return c.repo.NameWithOwner
	}
	return "fake-owner/fake-repo"
}

// matchesState reports whether a PR state satisfies the requested filter
// ("all" matches everything; otherwise case-insensitive equality against
// OPEN/CLOSED/MERGED).
func matchesState(prState, want string) bool {
	if want == "all" {
		return true
	}
	return equalFold(prState, want)
}

// bodyOf mirrors the SystemClient's BodyFile-wins-over-Body precedence so
// the fake records the same body the real client would send. A BodyFile
// path is recorded verbatim (the fake does not read the file).
func bodyOf(bodyFile, body string) string {
	if bodyFile != "" {
		return bodyFile
	}
	return body
}

// equalFold is a tiny ASCII case-insensitive compare; gh state strings are
// upper-case (OPEN) and callers pass lower-case (open), so we avoid pulling
// in strings just for this.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
