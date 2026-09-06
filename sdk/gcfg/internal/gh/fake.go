package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// Call is one request the fake was asked to make. Body is the decoded JSON
// request body, which is what an apply test asserts on.
type Call struct {
	Method string
	Path   string
	Body   map[string]any
}

// Fake is the Client every family, engine, and verb test runs against: it
// serves canned fixtures, records what was asked in order, and fails an
// unstubbed call loudly rather than silently returning nothing.
type Fake struct {
	mu     sync.Mutex
	single map[string]stubbed  // method+path → one response
	pages  map[string][]string // path → JSON array per page
	calls  []Call
}

type stubbed struct {
	status int
	body   string
	errMsg string
}

// NewFake returns an empty fake; stub it with Get/GetFile/GetPages/Fail.
func NewFake() *Fake {
	return &Fake{single: map[string]stubbed{}, pages: map[string][]string{}}
}

func key(method, path string) string { return method + " " + path }

// Get stubs one GET (or any method via Stub) with a status and JSON body.
func (f *Fake) Get(path string, status int, body string) { f.Stub("GET", path, status, body) }

// Stub stubs any method+path.
func (f *Fake) Stub(method, path string, status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.single[key(method, path)] = stubbed{status: status, body: body}
}

// GetFile stubs a GET from a fixture file — the usual shape for a family
// test, whose fixtures live in its own testdata/.
func (f *Fake) GetFile(t *testing.T, path, file string) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("fake fixture: %v", err)
	}
	f.Get(path, 200, string(b))
}

// GetPages stubs a paginated GET; each argument is one page's JSON array.
func (f *Fake) GetPages(path string, pages ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[path] = pages
}

// Fail stubs a failing response, the way GitHub answers a token that lacks
// a permission.
func (f *Fake) Fail(method, path string, status int, message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.single[key(method, path)] = stubbed{status: status, errMsg: message}
}

// Do implements Client.
func (f *Fake) Do(ctx context.Context, method, path string, body, out any) (int, error) {
	f.record(method, path, body)
	f.mu.Lock()
	s, ok := f.single[key(method, path)]
	f.mu.Unlock()
	if !ok {
		// A write with no stub is allowed and counts as accepted: apply
		// tests care about the request, not a canned answer.
		if method != "GET" {
			return 200, nil
		}
		return 0, fmt.Errorf("fake: no stub for %s %s", method, path)
	}
	if s.errMsg != "" {
		return s.status, fmt.Errorf("%s %s: HTTP %d %s", method, path, s.status, s.errMsg)
	}
	if out != nil && s.body != "" {
		if err := json.Unmarshal([]byte(s.body), out); err != nil {
			return s.status, fmt.Errorf("fake: decoding stub for %s %s: %w", method, path, err)
		}
	}
	return s.status, nil
}

// Paginate implements Client over the stubbed pages, falling back to a
// single stubbed array so a one-page family needs no special setup.
func (f *Fake) Paginate(ctx context.Context, path string, each func(json.RawMessage) error) error {
	f.record("GET", path, nil)
	f.mu.Lock()
	pages, ok := f.pages[path]
	if !ok {
		if s, single := f.single[key("GET", path)]; single {
			if s.errMsg != "" {
				f.mu.Unlock()
				return fmt.Errorf("GET %s: HTTP %d %s", path, s.status, s.errMsg)
			}
			pages, ok = []string{s.body}, true
		}
	}
	f.mu.Unlock()
	if !ok {
		return fmt.Errorf("fake: no stub for GET %s", path)
	}
	for _, page := range pages {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(page), &items); err != nil {
			return fmt.Errorf("fake: decoding page for %s: %w", path, err)
		}
		for _, item := range items {
			if err := each(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *Fake) record(method, path string, body any) {
	c := Call{Method: method, Path: path}
	if body != nil {
		if b, err := json.Marshal(body); err == nil {
			m := map[string]any{}
			if json.Unmarshal(b, &m) == nil {
				c.Body = m
			}
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}

// Calls returns every call in order — the record an engine test asserts on.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Writes returns only the calls that changed something.
func (f *Fake) Writes() []Call {
	var out []Call
	for _, c := range f.Calls() {
		if c.Method != "GET" {
			out = append(out, c)
		}
	}
	return out
}

// Body returns the request body of the last matching call.
func (f *Fake) Body(method, path string) map[string]any {
	calls := f.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method && calls[i].Path == path {
			return calls[i].Body
		}
	}
	return nil
}

// Paths returns "METHOD path" for every call, for a compact assertion on
// call order.
func (f *Fake) Paths() []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, strings.TrimSpace(c.Method+" "+c.Path))
	}
	return out
}
