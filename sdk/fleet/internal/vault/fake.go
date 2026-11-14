package vault

import (
	"context"
	"io"
	"sort"
	"strconv"
	"sync"
)

// Fake is an in-memory Store for tests, usable from any package.
// Calls records "<op> <name>" for each operation, never data.
type Fake struct {
	mu    sync.Mutex
	items map[string]Blob
	Calls []string
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{items: map[string]Blob{}} }

func (f *Fake) record(op, name string) { f.Calls = append(f.Calls, op+" "+name) }

func (f *Fake) Name() string                { return "fake" }
func (f *Fake) Ready(context.Context) error { return nil }
func (f *Fake) SignIn(context.Context, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (f *Fake) Get(_ context.Context, name string) (Blob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("get", name)
	b, ok := f.items[name]
	if !ok {
		return Blob{}, ErrNotFound
	}
	return Blob{Data: append(Secret(nil), b.Data...), Version: b.Version}, nil
}

func (f *Fake) Put(_ context.Context, name string, data Secret, version string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("put", name)
	cur, ok := f.items[name]
	next := 1
	if version == "" {
		if ok {
			return "", ErrConflict
		}
	} else {
		if !ok || cur.Version != version {
			return "", ErrConflict
		}
		n, _ := strconv.Atoi(cur.Version)
		next = n + 1
	}
	nv := strconv.Itoa(next)
	f.items[name] = Blob{Data: append(Secret(nil), data...), Version: nv}
	return nv, nil
}

func (f *Fake) List(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("list", "")
	names := make([]string, 0, len(f.items))
	for n := range f.items {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
